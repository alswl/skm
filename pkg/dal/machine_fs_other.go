//go:build !darwin && !linux

package dal

import "fmt"

// CreateMachineLink fails closed where the required path primitives are unavailable.
func CreateMachineLink(root, relative, source string) error {
	return fmt.Errorf("safe machine link creation is unavailable on this platform")
}
