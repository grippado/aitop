'use strict';

// aitop browser view. Consumes /api/stream (Server-Sent Events) — the exact
// same domain.Snapshot the TUI renders — and paints one card per live agent
// session. The honesty rule is load-bearing and mirrors the Go UI
// (internal/ui/panes/cards): a field the adapter didn't populate renders as
// "—", NEVER a fabricated 0. This file invents no data; it only reshapes the
// snapshot the backend already produced.

const DASH = '—';
const MAX_SPARK = 48; // points of per-session history kept, in memory, per tab

// Per-session sparkline history lives ONLY here, in the tab's memory. The
// server persists nothing; close the tab and it's gone. That's deliberate and
// honest — we show trend-since-you-opened-this, not a fabricated backfill.
const history = new Map(); // sessionID -> {ctx:[], tok:[]}

const el = {
  board: document.getElementById('board'),
  empty: document.getElementById('empty'),
  raw: document.getElementById('raw'),
  conn: document.getElementById('conn'),
  filterTool: document.getElementById('filter-tool'),
  sortCol: document.getElementById('sort-col'),
  toggleLayout: document.getElementById('toggle-layout'),
  toggleRaw: document.getElementById('toggle-raw'),
  warming: document.getElementById('warming'),
  sysCpu: document.getElementById('sys-cpu'),
  sysCpuVal: document.getElementById('sys-cpu-val'),
  sysMem: document.getElementById('sys-mem'),
  sysMemVal: document.getElementById('sys-mem-val'),
  sysNetVal: document.getElementById('sys-net-val'),
  sysTime: document.getElementById('sys-time'),
};

let lastSnapshot = null;
let showRaw = false;
// Layout mirrors the TUI's list/grid toggle ('v'). GRID packs cards into as
// many columns as fit (stretched to fill the row); LIST gives each card the
// full page width so the last action shows in full. Persisted per browser.
let layout = localStorage.getItem('aitop.layout') === 'list' ? 'list' : 'grid';

// ---- helpers mirroring the Go UI ---------------------------------------

const TOOL_NAMES = {
  'claude-code': 'claude code',
  'codex': 'codex',
  'cursor': 'cursor',
  'cursor-agent': 'cursor agent',
  'opencode': 'opencode',
};
function friendlyTool(tool) {
  if (TOOL_NAMES[tool]) return TOOL_NAMES[tool];
  return tool.replace(/^unknown:/, '');
}
function toolClass(tool) {
  if (TOOL_NAMES[tool]) return tool; // css var --tool-<name>
  return 'unknown';
}

function fmtTokens(n) {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
  if (n >= 1000) return Math.round(n / 1000) + 'k';
  return String(n);
}
function pctLabel(p) { return Math.round(p) + '%'; }

// gaugeColor mirrors theme.GaugeColor: <50 good, <80 warn, else bad.
function gaugeVar(p) {
  if (p >= 80) return 'var(--bad)';
  if (p >= 50) return 'var(--warn)';
  return 'var(--good)';
}

function lastPathSegment(path) {
  if (!path) return '';
  const trimmed = path.replace(/\/+$/, '');
  if (!trimmed) return '';
  const i = trimmed.lastIndexOf('/');
  return i < 0 ? trimmed : trimmed.slice(i + 1);
}
function fallbackTitle(s) {
  const proj = lastPathSegment(s.cwd);
  if (proj) return proj;
  return friendlyTool(s.tool) + ' session';
}

function ageSeconds(s) {
  if (!s.updated_at) return 0;
  const t = Date.parse(s.updated_at);
  if (Number.isNaN(t)) return 0;
  return Math.max(0, (Date.now() - t) / 1000);
}

function el2(tag, cls, text) {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text != null) n.textContent = text;
  return n;
}
function dashSpan() { return el2('span', 'dash', DASH); }

// ---- card model: port of cards.BuildCards ------------------------------

function buildCards(snap, toolFilter) {
  const sessions = snap.sessions || [];
  const usage = {};
  for (const u of (snap.usage || [])) usage[u.tool] = u;

  const procByPID = {};
  for (const p of (snap.processes || [])) procByPID[p.pid] = p;

  // cursor-agent and Cursor IDE share a composerId; drop the duplicate IDE
  // card (cursor-agent wins — its own transcript is the more precise source).
  const cursorAgentIDs = new Set();
  for (const s of sessions) {
    if (s.tool === 'cursor-agent' && s.id) cursorAgentIDs.add(s.id);
  }

  // representative session per tool absorbs unattributable processes.
  const rep = {};
  for (const s of sessions) {
    if (toolFilter && s.tool !== toolFilter) continue;
    const cur = rep[s.tool];
    if (!cur || betterRep(s, cur)) rep[s.tool] = s;
  }
  const matchedPID = new Set();
  for (const s of sessions) {
    if (s.pid && procByPID[s.pid]) matchedPID.add(s.pid);
  }
  const leftoverCount = {}, leftoverCPU = {};
  for (const p of (snap.processes || [])) {
    if (matchedPID.has(p.pid)) continue;
    leftoverCount[p.tool] = (leftoverCount[p.tool] || 0) + 1;
    leftoverCPU[p.tool] = (leftoverCPU[p.tool] || 0) + (p.cpu_pct || 0);
  }

  const labelByPID = {};
  for (const s of sessions) {
    if (s.pid) labelByPID[s.pid] = s.title || s.id;
  }

  const cards = [];
  for (const s of sessions) {
    if (toolFilter && s.tool !== toolFilter) continue;
    if (!s.alive) continue; // dead sessions get no card
    if (s.tool === 'cursor' && s.id && cursorAgentIDs.has(s.id)) continue;

    const hasTokens = (s.tokens_in || 0) > 0 || (s.tokens_out || 0) > 0;
    const hasContext = (s.context_used_pct || 0) > 0;
    const u = usage[s.tool];
    const hasCost = !!(u && u.available);

    let procCount = 0, procCPU = 0;
    if (s.pid && procByPID[s.pid]) { procCount++; procCPU += procByPID[s.pid].cpu_pct || 0; }
    const r = rep[s.tool];
    if (r && r.id === s.id && r.pid === s.pid) {
      procCount += leftoverCount[s.tool] || 0;
      procCPU += leftoverCPU[s.tool] || 0;
    }

    // parent resolves to a board label only when the parent PID is itself a
    // session on the board — otherwise "spawned (parent not on board)".
    let parentLabel = '';
    if (s.parent_pid) parentLabel = labelByPID[s.parent_pid] || '';

    cards.push({
      tool: s.tool,
      id: s.id,
      pid: s.pid || 0,
      status: s.status,
      cwd: s.cwd || '',
      model: s.model || '',
      title: s.title || '',
      lastAction: s.last_action || '',
      ageSec: ageSeconds(s),
      kind: s.kind || '',
      parentPid: s.parent_pid || 0,
      parentLabel,
      hasContext, contextPct: s.context_used_pct || 0,
      hasTokens, tokensIn: s.tokens_in || 0, tokensOut: s.tokens_out || 0,
      hasCost,
      limit5: u ? u.limit_five_hour : null,
      limit7: u ? u.limit_weekly : null,
      procCount, procCPU,
    });
  }
  return cards;
}

function betterRep(a, b) {
  if (a.alive !== b.alive) return a.alive;
  return Date.parse(a.updated_at || 0) > Date.parse(b.updated_at || 0);
}

function sortCards(cards, col) {
  const by = {
    tokens: (a, b) => (b.tokensIn + b.tokensOut) - (a.tokensIn + a.tokensOut),
    age: (a, b) => b.ageSec - a.ageSec,
    tool: (a, b) => a.tool.localeCompare(b.tool),
    context: (a, b) => b.contextPct - a.contextPct,
  };
  cards.sort(by[col] || by.context);
}

// ---- sparkline ----------------------------------------------------------

function sparkline(values, w, h, stroke) {
  if (!values || values.length < 2) return null;
  const max = Math.max(...values, 1);
  const min = Math.min(...values, 0);
  const span = max - min || 1;
  const stepX = w / (values.length - 1);
  const pts = values.map((v, i) => {
    const x = i * stepX;
    const y = h - ((v - min) / span) * (h - 2) - 1;
    return x.toFixed(1) + ',' + y.toFixed(1);
  }).join(' ');
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('width', w); svg.setAttribute('height', h);
  svg.setAttribute('class', 'metric-spark');
  const poly = document.createElementNS('http://www.w3.org/2000/svg', 'polyline');
  poly.setAttribute('points', pts);
  poly.setAttribute('fill', 'none');
  poly.setAttribute('stroke', stroke);
  poly.setAttribute('stroke-width', '1.3');
  svg.appendChild(poly);
  return svg;
}

// ---- agentic suggestion (read-only: copy, never execute) ---------------

// Point at a "—" an /aitop-enhance pass could fill, or an unknown tool an
// /aitop-adapter run could cover — and hand over the ready command. We only
// COPY it: running it is a human's job in Claude Code. This is the honest way
// to "induce evolution" without inverting the read-only invariant.
function suggestionFor(c) {
  if (c.tool.startsWith('unknown:')) {
    const name = c.tool.replace(/^unknown:/, '');
    return { cmd: `/aitop-adapter ${name}`, why: `no dedicated adapter for ${name} yet` };
  }
  if (!c.model) return { cmd: `/aitop-enhance ${c.tool}`, why: 'model shows —' };
  if (!c.hasContext) return { cmd: `/aitop-enhance ${c.tool}`, why: 'context % shows —' };
  return null;
}

async function copyCmd(text, btn) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    const ta = document.createElement('textarea');
    ta.value = text; document.body.appendChild(ta); ta.select();
    try { document.execCommand('copy'); } catch {}
    ta.remove();
  }
  const old = btn.textContent;
  btn.textContent = 'copied ✓';
  btn.classList.add('copied');
  setTimeout(() => { btn.textContent = old; btn.classList.remove('copied'); }, 1400);
}

// ---- render -------------------------------------------------------------

// A card is split into two zones so the CSS can re-flow them per layout:
//   .card-main — title, badges, tool/cwd, last action (the "what")
//   .card-side — context, tokens, usage, suggestion (the "how much")
// In GRID they stack (one narrow column); in LIST they sit side by side so the
// last action gets the full page width and shows in full. Same DOM either way.
function renderCard(c, onBoardPids) {
  const card = el2('div', 'card');
  card.style.setProperty('--tool', `var(--tool-${toolClass(c.tool)})`);
  const nested = c.parentPid && onBoardPids.has(c.parentPid);
  if (nested) card.classList.add('card--child');

  const main = el2('div', 'card-main');
  const side = el2('div', 'card-side');

  // header: title + lineage
  const head = el2('div', 'card-head');
  const title = el2('div', 'card-title', c.title || fallbackTitle(c));
  if (!c.title) title.classList.add('card-title--fallback');
  head.appendChild(title);
  const lineage = lineageText(c);
  if (lineage) head.appendChild(el2('span', 'lineage', lineage));
  main.appendChild(head);

  // pill row: state badges + tool(model) pill + cwd
  const meta = el2('div', 'card-meta');
  const [badgeText, badgeCls] = stateBadge(c.status);
  meta.appendChild(el2('span', `badge ${badgeCls}`, badgeText));
  if (c.kind) meta.appendChild(el2('span', 'badge badge--kind', `[${c.kind}]`));
  const pill = el2('span', 'pill');
  pill.appendChild(document.createTextNode(friendlyTool(c.tool)));
  if (c.model) {
    pill.appendChild(document.createTextNode(' '));
    pill.appendChild(el2('span', 'model', `(${c.model})`));
  }
  meta.appendChild(pill);
  meta.appendChild(c.cwd ? el2('span', 'cwd', c.cwd) : dashSpan());
  main.appendChild(meta);

  // last action — the session's own last tool call / message
  const action = el2('div', 'action');
  if (c.lastAction) { action.textContent = c.lastAction; }
  else { action.classList.add('action--empty'); action.textContent = DASH; }
  main.appendChild(action);

  // right zone: context, tokens, usage, suggestion
  side.appendChild(contextMetric(c));
  side.appendChild(tokensMetric(c));

  const usage = el2('div', 'usage');
  usage.textContent = usageText(c);
  side.appendChild(usage);

  const sug = suggestionFor(c);
  if (sug) {
    const wrap = el2('div', 'suggest');
    wrap.appendChild(document.createTextNode(sug.why + ' →'));
    const btn = el2('button', null, `copy ${sug.cmd}`);
    btn.title = 'Copy this command, then run it in Claude Code (aitop never runs it for you)';
    btn.addEventListener('click', () => copyCmd(sug.cmd, btn));
    wrap.appendChild(btn);
    side.appendChild(wrap);
  }

  card.appendChild(main);
  card.appendChild(side);
  return card;
}

function lineageText(c) {
  const parts = [];
  if (c.parentPid) {
    parts.push(c.parentLabel ? `▸ spawned by ${c.parentLabel}` : '▸ spawned (parent not on board)');
  }
  return parts.join(' ');
}

function stateBadge(status) {
  if (status === 'busy') return ['● running', 'badge--running'];
  if (status === 'idle') return ['◌ idle', 'badge--idle'];
  return ['◌ unknown', 'badge--unknown'];
}

function contextMetric(c) {
  const m = el2('div', 'metric');
  m.appendChild(el2('span', 'metric-label', 'Context'));
  const body = el2('div', 'metric-body');
  if (c.hasContext) {
    const meter = el2('div', 'meter');
    const fill = el2('div', 'fill');
    fill.style.width = Math.min(100, c.contextPct) + '%';
    fill.style.background = gaugeVar(c.contextPct);
    meter.appendChild(fill);
    body.appendChild(meter);
    m.appendChild(body);
    let label = pctLabel(c.contextPct);
    if (c.hasTokens) {
      const total = Math.round(c.tokensIn * 100 / c.contextPct);
      label = `${fmtTokens(c.tokensIn)}/${fmtTokens(total)} (${pctLabel(c.contextPct)})`;
    }
    m.appendChild(el2('span', 'metric-val', label));
    const spark = sparkline((history.get(c.id) || {}).ctx, 52, 16, gaugeVar(c.contextPct));
    if (spark) m.appendChild(spark);
  } else {
    m.appendChild(body);
    m.appendChild(dashSpan());
  }
  return m;
}

function tokensMetric(c) {
  const m = el2('div', 'metric');
  m.appendChild(el2('span', 'metric-label', 'Tokens'));
  const body = el2('div', 'metric-body');
  if (c.hasTokens) {
    const tok = el2('div', 'tok');
    tok.appendChild(el2('span', 'in', `IN ↑ ${fmtTokens(c.tokensIn)}`));
    tok.appendChild(el2('span', 'out', `OUT ↓ ${fmtTokens(c.tokensOut)}`));
    body.appendChild(tok);
    m.appendChild(body);
    const spark = sparkline((history.get(c.id) || {}).tok, 52, 16, 'var(--tok-in)');
    if (spark) m.appendChild(spark);
  } else {
    m.appendChild(body);
    m.appendChild(dashSpan());
  }
  return m;
}

function usageText(c) {
  if (!c.hasCost) return 'usage: ' + DASH;
  const l5 = c.limit5 != null ? Math.round(c.limit5) + '%' : DASH;
  const l7 = c.limit7 != null ? Math.round(c.limit7) + '%' : DASH;
  const procs = c.procCount > 0 ? `${c.procCount} procs, ${Math.round(c.procCPU)}% CPU summed` : DASH;
  return `5h ${l5} · 7d ${l7} · ${procs}`;
}

// ---- history bookkeeping ------------------------------------------------

function recordHistory(cards) {
  const live = new Set();
  for (const c of cards) {
    live.add(c.id);
    let h = history.get(c.id);
    if (!h) { h = { ctx: [], tok: [] }; history.set(c.id, h); }
    if (c.hasContext) { h.ctx.push(c.contextPct); if (h.ctx.length > MAX_SPARK) h.ctx.shift(); }
    if (c.hasTokens) { h.tok.push(c.tokensIn + c.tokensOut); if (h.tok.length > MAX_SPARK) h.tok.shift(); }
  }
  // forget sessions that have left the board, so the map can't grow forever.
  for (const id of history.keys()) if (!live.has(id)) history.delete(id);
}

// ---- top-level render ---------------------------------------------------

function render() {
  if (!lastSnapshot) return;
  const snap = lastSnapshot;
  const toolFilter = el.filterTool.value;

  populateToolFilter(snap);

  const cards = buildCards(snap, toolFilter);
  recordHistory(cards);
  sortCards(cards, el.sortCol.value);

  const onBoardPids = new Set(cards.filter(c => c.pid).map(c => c.pid));

  el.board.classList.toggle('board--list', layout === 'list');
  el.board.classList.toggle('board--grid', layout === 'grid');
  el.board.replaceChildren();
  if (cards.length === 0) {
    el.empty.hidden = false;
  } else {
    el.empty.hidden = true;
    for (const c of cards) el.board.appendChild(renderCard(c, onBoardPids));
  }

  renderFooter(snap);

  if (showRaw) el.raw.textContent = JSON.stringify(snap, null, 2);
}

function populateToolFilter(snap) {
  const tools = [...new Set((snap.tools || []).map(t => t.tool))].sort();
  const cur = el.filterTool.value;
  const want = ['', ...tools];
  const have = [...el.filterTool.options].map(o => o.value);
  if (want.join('|') === have.join('|')) return; // unchanged, keep selection
  el.filterTool.replaceChildren();
  el.filterTool.appendChild(new Option('all', ''));
  for (const t of tools) el.filterTool.appendChild(new Option(friendlyTool(t), t));
  el.filterTool.value = want.includes(cur) ? cur : '';
}

function renderFooter(snap) {
  el.warming.hidden = !snap.warming;
  const sys = snap.system || {};
  const cores = sys.per_core_cpu_pct || [];
  const cpuAvg = cores.length ? cores.reduce((a, b) => a + b, 0) / cores.length : 0;
  setMeter(el.sysCpu, cpuAvg, gaugeVar(cpuAvg));
  el.sysCpuVal.textContent = cores.length ? `${Math.round(cpuAvg)}% · ${cores.length} cores` : DASH;

  const memPct = sys.mem_total_mb > 0 ? (sys.mem_used_mb / sys.mem_total_mb) * 100 : 0;
  setMeter(el.sysMem, memPct, 'var(--muted)');
  el.sysMemVal.textContent = sys.mem_total_mb > 0
    ? `${fmtMB(sys.mem_used_mb)}/${fmtMB(sys.mem_total_mb)} (${Math.round(memPct)}%)` : DASH;

  el.sysNetVal.textContent = (sys.net_up_bps != null)
    ? `↑ ${fmtBps(sys.net_up_bps)}  ↓ ${fmtBps(sys.net_down_bps)}` : DASH;

  el.sysTime.textContent = snap.taken_at ? new Date(snap.taken_at).toLocaleTimeString() : DASH;
}

function setMeter(meter, pct, color) {
  let fill = meter.querySelector('.fill');
  if (!fill) { fill = el2('div', 'fill'); meter.appendChild(fill); }
  fill.style.width = Math.min(100, Math.max(0, pct)) + '%';
  fill.style.background = color;
}
function fmtMB(mb) {
  if (mb >= 1024) return (mb / 1024).toFixed(1) + 'G';
  return Math.round(mb) + 'M';
}
function fmtBps(bps) {
  if (bps >= 1e6) return (bps / 1e6).toFixed(1) + ' MB/s';
  if (bps >= 1e3) return (bps / 1e3).toFixed(0) + ' kB/s';
  return Math.round(bps) + ' B/s';
}

// ---- wiring -------------------------------------------------------------

function applyLayoutButton() {
  el.toggleLayout.textContent = layout === 'list' ? '▤ list' : '▦ grid';
  el.toggleLayout.setAttribute('aria-pressed', String(layout === 'list'));
}
applyLayoutButton();

el.filterTool.addEventListener('change', render);
el.sortCol.addEventListener('change', render);
el.toggleLayout.addEventListener('click', () => {
  layout = layout === 'grid' ? 'list' : 'grid';
  localStorage.setItem('aitop.layout', layout);
  applyLayoutButton();
  render();
});
el.toggleRaw.addEventListener('click', () => {
  showRaw = !showRaw;
  el.toggleRaw.setAttribute('aria-pressed', String(showRaw));
  el.raw.hidden = !showRaw;
  if (showRaw && lastSnapshot) el.raw.textContent = JSON.stringify(lastSnapshot, null, 2);
});

function setConn(state, text) {
  el.conn.className = 'conn conn--' + state;
  el.conn.textContent = text;
}

function connect() {
  const es = new EventSource('/api/stream');
  es.onopen = () => setConn('ok', '● live');
  es.onmessage = (ev) => {
    try { lastSnapshot = JSON.parse(ev.data); } catch { return; }
    render();
  };
  es.onerror = () => {
    // EventSource auto-reconnects; reflect the gap honestly meanwhile.
    setConn('down', '● reconnecting');
  };
}

connect();
