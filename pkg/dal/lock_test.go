package dal

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSyncLockCrashRelease(t *testing.T) {
	if root := os.Getenv("SKM_TEST_CRASH_LOCK"); root != "" {
		lock, err := AcquireLock(context.Background(), root)
		if err != nil {
			os.Exit(3)
		}
		_ = lock // deliberately leave descriptor open until process exit
		os.Exit(17)
	}
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestSyncLockCrashRelease$")
	cmd.Env = append(os.Environ(), "SKM_TEST_CRASH_LOCK="+root)
	err := cmd.Run()
	require.Error(t, err)
	require.Equal(t, 17, cmd.ProcessState.ExitCode())
	lock, err := AcquireLockWait(context.Background(), root, time.Second)
	require.NoError(t, err)
	_, err = AcquireLockWait(context.Background(), root, 20*time.Millisecond)
	require.Error(t, err)
	lock.Release()
	again, err := AcquireLockWait(context.Background(), root, time.Second)
	require.NoError(t, err)
	again.Release()
}
