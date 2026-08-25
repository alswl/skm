package commands

import (
	"context"

	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/services"
)

var archiveCmd = newLifecycleCommand("archive NAME", "Move an active entry into the archive",
	func(ctx context.Context, s *services.Services, name string, o engines.LifecycleOptions) (*services.LifecycleResult, error) {
		return s.Archive(ctx, name, o)
	})

func init() {
	rootCmd.AddCommand(archiveCmd)
}
