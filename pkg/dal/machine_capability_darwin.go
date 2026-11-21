//go:build darwin

package dal

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// macOS reports per-volume support for RENAME_EXCL through fgetattrlist.
func machineCheckExclusiveRename(fd int) error {
	attrs := unix.Attrlist{Bitmapcount: 5, Volattr: unix.ATTR_VOL_CAPABILITIES}
	var result [36]byte // length followed by capabilities[4] and valid[4]
	// x/sys does not expose fgetattrlist; failure of this read-only query blocks unlink.
	_, _, errno := unix.Syscall6(unix.SYS_FGETATTRLIST, uintptr(fd), uintptr(unsafe.Pointer(&attrs)), uintptr(unsafe.Pointer(&result[0])), uintptr(len(result)), 0, 0) //nolint:staticcheck
	if errno != 0 {
		return fmt.Errorf("query exclusive rename capability: %w", errno)
	}
	if !machineHasExclusiveRename(result) {
		return fmt.Errorf("filesystem does not report exclusive rename support")
	}
	return nil
}

func machineHasExclusiveRename(result [36]byte) bool {
	const renameExclusive = 0x00080000 // VOL_CAP_INT_RENAME_EXCL
	return binary.NativeEndian.Uint32(result[:4]) >= uint32(len(result)) && binary.NativeEndian.Uint32(result[24:28])&renameExclusive != 0 && binary.NativeEndian.Uint32(result[8:12])&renameExclusive != 0
}
