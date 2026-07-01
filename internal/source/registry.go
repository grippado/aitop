package source

import "context"

// Resolve returns every registered Source whose Detect() reports true on
// this machine. Order is stable: claude-code, codex, cursor, then any
// fallback sources appended last so dedicated adapters take precedence.
func Resolve(ctx context.Context, all []Source) []Source {
	var detected []Source
	for _, s := range all {
		if s.Detect(ctx) {
			detected = append(detected, s)
		}
	}
	return detected
}
