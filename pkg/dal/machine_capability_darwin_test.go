//go:build darwin

package dal

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMachineExclusiveRenameCapabilityRequiresValidSupportedBit(t *testing.T) {
	var result [36]byte
	binary.NativeEndian.PutUint32(result[:4], uint32(len(result)))
	require.False(t, machineHasExclusiveRename(result))
	binary.NativeEndian.PutUint32(result[8:12], 0x80000)
	require.False(t, machineHasExclusiveRename(result))
	binary.NativeEndian.PutUint32(result[24:28], 0x80000)
	require.True(t, machineHasExclusiveRename(result))
	binary.NativeEndian.PutUint32(result[:4], 8)
	require.False(t, machineHasExclusiveRename(result))
}
