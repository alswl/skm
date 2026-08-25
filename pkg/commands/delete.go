package commands

import (
	"context"

	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/services"
)

var deleteCmd = newLifecycleCommand("delete NAME", "Permanently remove an entry (requires --force)",
	func(ctx context.Context, s *services.Services, name string, o engines.LifecycleOptions) (*services.LifecycleResult, error) {
		return s.Delete(ctx, name, o)
	})

func init() {
	rootCmd.AddCommand(deleteCmd)
}
