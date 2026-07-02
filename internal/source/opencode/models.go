package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// modelsCache reads opencode's own models catalog cache
// (~/.cache/opencode/models.json, confirmed ~3MB real file on this
// machine) for a real context-window size and a real display name per
// provider+model — the same authoritative-source-over-guessing preference
// Codex's adapter gets for free from its own token_count event and
// Claude's adapter has to fake with a hardcoded 1M constant / a hand-
// written model-id shortener. Best-effort and optional: opencode may not
// have fetched this cache yet on a fresh install, in which case
// contextWindow/friendlyName simply report not-found and the caller omits
// ContextUsedPct/Model — an honest gap, not a guess.
type modelsCache struct {
	home string

	mu      sync.Mutex
	loaded  bool
	windows map[string]int64  // "providerID/modelID" -> context window
	names   map[string]string // "providerID/modelID" -> display name, lowercased
}

func newModelsCache(home string) *modelsCache {
	return &modelsCache{home: home}
}

// modelsFile mirrors the top-level shape of models.json: a map of
// providerID -> provider, each carrying its own map of modelID -> model.
type modelsFile map[string]struct {
	Models map[string]struct {
		Name  string `json:"name"`
		Limit struct {
			Context int64 `json:"context"`
		} `json:"limit"`
	} `json:"models"`
}

func (c *modelsCache) path() string {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, "opencode", "models.json")
	}
	return filepath.Join(c.home, ".cache", "opencode", "models.json")
}

func (c *modelsCache) ensureLoaded() {
	if c.loaded {
		return
	}
	c.windows, c.names = c.load()
	c.loaded = true
}

func (c *modelsCache) contextWindow(providerID, modelID string) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoaded()
	w, ok := c.windows[providerID+"/"+modelID]
	return w, ok
}

// friendlyName returns opencode's own display name for a model (e.g.
// "DeepSeek V4 Flash Free"), lowercased to match the short/lowercase style
// every other adapter's model label already uses (e.g. Claude's "opus
// 4.8").
func (c *modelsCache) friendlyName(providerID, modelID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoaded()
	n, ok := c.names[providerID+"/"+modelID]
	return n, ok
}

func (c *modelsCache) load() (windows map[string]int64, names map[string]string) {
	raw, err := reader.ReadFile(c.path())
	if err != nil {
		return nil, nil
	}
	var mf modelsFile
	if err := json.Unmarshal(raw, &mf); err != nil {
		return nil, nil
	}
	windows = map[string]int64{}
	names = map[string]string{}
	for providerID, provider := range mf {
		for modelID, model := range provider.Models {
			key := providerID + "/" + modelID
			if model.Limit.Context > 0 {
				windows[key] = model.Limit.Context
			}
			if model.Name != "" {
				names[key] = strings.ToLower(model.Name)
			}
		}
	}
	return windows, names
}
