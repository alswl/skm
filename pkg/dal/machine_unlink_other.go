//go:build !darwin && !linux

package dal

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
)

func InspectMachineLink(root, relative string) (MachineLinkIdentity, error) {
	return MachineLinkIdentity{}, fmt.Errorf("%w: safe link inspection is unavailable on this platform", common.ErrMachineRenameUnavailable)
}

func RemoveMachineLink(root, relative string, expected MachineLinkIdentity) error {
	return fmt.Errorf("safe link removal is unavailable on this platform")
}
