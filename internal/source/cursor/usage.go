package cursor

import (
	"context"

	"github.com/grippado/aitop/internal/domain"
)

// Usage is always unavailable for Cursor: cost/token accounting is
// proprietary and cloud-side, nothing about it is observable locally.
// Available:false here, never a fabricated $0 that could be misread as
// "confirmed zero spend."
func (a *Adapter) Usage(ctx context.Context) (domain.UsageInfo, error) {
	return domain.UsageInfo{Tool: Name, Available: false}, nil
}
