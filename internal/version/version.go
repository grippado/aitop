// Package version holds the build-time version string, injected via
// -ldflags "-X github.com/grippado/aitop/internal/version.Version=...".
package version

// Version defaults to "dev" for local builds; goreleaser overrides it.
var Version = "dev"
