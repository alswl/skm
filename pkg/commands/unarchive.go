package commands

import (
	"context"

	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/services"
)

var unarchiveCmd = newLifecycleCommand("unarchive NAME", "Restore an archived entry",
	func(ctx context.Context, s *services.Services, name string, o engines.LifecycleOptions) (*services.LifecycleResult, error) {
		return s.Unarchive(ctx, name, o)
	})

func init() {
	rootCmd.AddCommand(unarchiveCmd)
}
