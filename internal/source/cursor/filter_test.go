package cursor

import "testing"

func TestIsCursorIDEProcess(t *testing.T) {
	cases := []struct {
		name, exe string
		want      bool
	}{
		{"Cursor", "/Applications/Cursor.app/Contents/MacOS/Cursor", true},
		{"Cursor Helper", "/Applications/Cursor.app/Contents/Frameworks/Cursor Helper.app/Contents/MacOS/Cursor Helper", true},
		{"Cursor Helper (Renderer)", "", true},
		// Real false positives observed in practice on this machine.
		{"CursorUIViewService", "/System/Library/PrivateFrameworks/TextInputUIMacHelper.framework/Versions/A/XPCServices/CursorUIViewService.xpc/Contents/MacOS/CursorUIViewService", false},
		{"cursor-agent", "/Users/grippado/.local/bin/cursor-agent", false},
		{"node", "/Users/grippado/.local/share/cursor-agent/versions/x/node", false},
	}
	for _, c := range cases {
		if got := isCursorIDEProcess(c.name, c.exe); got != c.want {
			t.Errorf("isCursorIDEProcess(%q, %q) = %v, want %v", c.name, c.exe, got, c.want)
		}
	}
}

func TestIsCursorOwnProcess(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"/Applications/Cursor.app/Contents/MacOS/Cursor", true},
		{"/Applications/Cursor.app/Contents/Frameworks/Cursor Helper.app/Contents/MacOS/Cursor Helper", true},
		{"Cursor Helper: mcp-process", true},
		{"Cursor Helper: fileWatcher [1:empty-window]", true},
		{"Cursor Helper: shared-process", true},
		{"Cursor Helper: terminal pty-host", true},
		{"Cursor Helper (Plugin): extension-host  backoffice [1-5]", true},
		{"Cursor Helper (Renderer)", true},
		// Real descendants of Cursor's integrated terminal, observed on
		// this machine's actual process-monitor log — must be excluded,
		// they are the user's own tools, not Cursor's consumption.
		{"/bin/zsh", false},
		{"/bin/sh", false},
		{"ssh", false},
		{"kubectl", false},
		{"docker", false},
		{"tee", false},
		{"/Library/Developer/CommandLineTools/usr/bin/git", false},
		{"/usr/bin/git", false},
	}
	for _, c := range cases {
		if got := isCursorOwnProcess(c.name); got != c.want {
			t.Errorf("isCursorOwnProcess(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestExtensionHostWorkspace(t *testing.T) {
	cases := []struct {
		name      string
		wantLabel string
		wantRank  int
		wantOK    bool
	}{
		{"Cursor Helper (Plugin): extension-host  empty [1-1]", "", 0, false},
		{"Cursor Helper (Plugin): extension-host  grippado [1-2]", "grippado", 2, true},
		{"Cursor Helper (Plugin): extension-host  Desktop [1-4]", "Desktop", 4, true},
		{"Cursor Helper (Plugin): extension-host  backoffice [1-5]", "backoffice", 5, true},
		{"Cursor Helper: mcp-process", "", 0, false},
	}
	for _, c := range cases {
		label, rank, ok := extensionHostWorkspace(c.name)
		if ok != c.wantOK || label != c.wantLabel || (ok && rank != c.wantRank) {
			t.Errorf("extensionHostWorkspace(%q) = (%q, %d, %v), want (%q, %d, %v)", c.name, label, rank, ok, c.wantLabel, c.wantRank, c.wantOK)
		}
	}
}

func TestIngest_FiltersNonCursorDescendantsAndPicksHighestRankedWorkspace(t *testing.T) {
	a := New()
	data := []byte(
		`{"sessionId":"s1","rows":[` +
			`{"pid":1,"ppid":0,"processName":"/Applications/Cursor.app/Contents/MacOS/Cursor","sampleAvgMemMb":250,"cpuDuringSamplePeakPct":1},` +
			`{"pid":2,"ppid":1,"processName":"Cursor Helper (Plugin): extension-host  grippado [1-2]","sampleAvgMemMb":240,"cpuDuringSamplePeakPct":2},` +
			`{"pid":3,"ppid":1,"processName":"Cursor Helper (Plugin): extension-host  backoffice [1-5]","sampleAvgMemMb":655,"cpuDuringSamplePeakPct":47},` +
			`{"pid":4,"ppid":3,"processName":"kubectl","sampleAvgMemMb":34,"cpuDuringSamplePeakPct":0},` +
			`{"pid":5,"ppid":3,"processName":"docker","sampleAvgMemMb":20,"cpuDuringSamplePeakPct":0},` +
			`{"pid":6,"ppid":3,"processName":"/bin/zsh","sampleAvgMemMb":6,"cpuDuringSamplePeakPct":0},` +
			`{"pid":7,"ppid":3,"processName":"/usr/bin/git","sampleAvgMemMb":1,"cpuDuringSamplePeakPct":0}` +
			`]}` + "\n")

	if !a.ingest(data) {
		t.Fatal("expected ingest to report parsed data")
	}

	if len(a.lastRows) != 3 {
		t.Fatalf("expected only the 3 real Cursor rows (main + 2 extension-hosts), got %d: %+v", len(a.lastRows), a.lastRows)
	}
	for _, excludedPID := range []int{4, 5, 6, 7} {
		if _, ok := a.lastRows[excludedPID]; ok {
			t.Errorf("pid %d (terminal descendant) should have been filtered out", excludedPID)
		}
	}

	if got := a.lastCWD["s1"]; got != "backoffice" {
		t.Fatalf("expected the higher-ranked workspace [1-5] (backoffice) to win over [1-2] (grippado), got %q", got)
	}
}
