//go:build darwin

package dal

import "golang.org/x/sys/unix"

func machineRenameExclusive(fromFD int, from string, toFD int, to string) error {
	return unix.RenameatxNp(fromFD, from, toFD, to, unix.RENAME_EXCL)
}
