package commands

import (
	"context"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/services"
)

var toCommandCmd = newLifecycleCommand("to-command NAME", "Convert a directory skill into a command",
	func(ctx context.Context, s *services.Services, name string, o engines.LifecycleOptions) (*services.LifecycleResult, error) {
		return s.Convert(ctx, name, common.KindCommand, o)
	})

func init() {
	rootCmd.AddCommand(toCommandCmd)
}
