package services

import (
	"context"

	"github.com/alswl/skm/skm/pkg/installer"
)

// OrphanDangling finds dangling target objects that cannot be repaired by
// reinstalling an active repository entry of the same name.
func (s *Services) OrphanDangling(ctx context.Context) ([]installer.DanglingInstall, error) {
	return s.Installer.OrphanDangling(ctx, s.Scan())
}

// CleanDangling removes one orphaned link only after rechecking that it is
// still a symlink and still unresolved. It never removes a real user file.
func (s *Services) CleanDangling(ctx context.Context, item installer.DanglingInstall) error {
	return s.Installer.CleanDangling(ctx, item)
}
