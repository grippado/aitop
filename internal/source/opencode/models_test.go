package opencode

import (
	"path/filepath"
	"testing"
)

func TestModelsCache_ContextWindowAndFriendlyName(t *testing.T) {
	home := "/home/test"
	path := filepath.Join(home, ".cache", "opencode", "models.json")
	data := `{
		"opencode": {
			"models": {
				"deepseek-v4-flash-free": {"name": "DeepSeek V4 Flash Free", "limit": {"context": 200000}}
			}
		}
	}`
	withFakeReader(t, &fakeReader{files: map[string][]byte{path: []byte(data)}})

	c := newModelsCache(home)
	window, ok := c.contextWindow("opencode", "deepseek-v4-flash-free")
	if !ok || window != 200000 {
		t.Fatalf("contextWindow = (%v, %v), want (200000, true)", window, ok)
	}
	if _, ok := c.contextWindow("opencode", "unknown-model"); ok {
		t.Fatalf("expected ok=false for an unknown model")
	}

	name, ok := c.friendlyName("opencode", "deepseek-v4-flash-free")
	if !ok || name != "deepseek v4 flash free" {
		t.Fatalf("friendlyName = (%q, %v), want (%q, true)", name, ok, "deepseek v4 flash free")
	}
}

func TestModelsCache_MissingFileIsHonestlyNotFound(t *testing.T) {
	withFakeReader(t, &fakeReader{})
	c := newModelsCache("/home/test")
	if _, ok := c.contextWindow("opencode", "deepseek-v4-flash-free"); ok {
		t.Fatalf("expected ok=false when models.json doesn't exist")
	}
	if _, ok := c.friendlyName("opencode", "deepseek-v4-flash-free"); ok {
		t.Fatalf("expected ok=false when models.json doesn't exist")
	}
}

func TestModelsCache_LoadsOnceAndCaches(t *testing.T) {
	withFakeReader(t, &fakeReader{})
	c := newModelsCache("/home/test")
	c.contextWindow("p", "m") // first call: attempts a load, finds nothing, caches empty result
	if !c.loaded {
		t.Fatalf("expected loaded=true after the first lookup")
	}
}
