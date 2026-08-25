package commands

import (
	"context"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/services"
)

var toSkillCmd = newLifecycleCommand("to-skill NAME", "Convert a directory command into a skill",
	func(ctx context.Context, s *services.Services, name string, o engines.LifecycleOptions) (*services.LifecycleResult, error) {
		return s.Convert(ctx, name, common.KindSkill, o)
	})

func init() {
	rootCmd.AddCommand(toSkillCmd)
}
