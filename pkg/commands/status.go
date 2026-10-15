package commands

import (
	"fmt"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

// statusReport is the single-entry health report (contract/cli-json.md). The
// CLI always reports modified as "unknown".
type statusReport struct {
	Sync            *engines.SyncStatus `json:"sync,omitempty"`
	SyncError       string              `json:"sync_error,omitempty"`
	Path            string              `json:"path"`
	Status          common.Status       `json:"status"`
	Address         *string             `json:"address"`
	Installed       bool                `json:"installed"`
	LinkHealthy     bool                `json:"link_healthy"`
	DanglingTargets []string            `json:"dangling_targets"`
	Modified        string              `json:"modified"`
	Version         *string             `json:"version"`
	Error           *string             `json:"error"`
}

var statusCmd = &cobra.Command{
	Use:     "status NAME",
	Short:   "Show the health of a single entry across targets",
	Example: "  skm status review --json",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		entry, err := svc.ResolveEntry(args[0])
		if err != nil {
			return fmt.Errorf("status: %w", err)
		}
		if entry == nil {
			return common.WithExitCode(fmt.Errorf("status: entry %q not found", args[0]), common.ExitObject)
		}
		rep := buildStatusReport(svc, entry)
		if !flagJSON && rep.Sync != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "mode=%s customization=%s baseline=%s baseline_state=%s\nupstream=%s\nfork=%s\nattempt=%s operation=%s/%s rollback=%v recovery_required=%v cleanup_pending=%v next_actions=%s\n", rep.Sync.Mode, rep.Sync.Customization, rep.Sync.BaselineVersion, rep.Sync.BaselineState, rep.Sync.UpstreamPath, rep.Sync.ForkPath, rep.Sync.AttemptPath, rep.Sync.OperationID, rep.Sync.OperationState, rep.Sync.RollbackAvailable, rep.Sync.RecoveryRequired, rep.Sync.CleanupPending, strings.Join(rep.Sync.NextActions, ","))
		}
		if !flagJSON && rep.SyncError != "" {
			fmt.Fprintln(cmd.OutOrStdout(), rep.SyncError)
		}
		if flagJSON {
			return printJSON(cmd, rep)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s: %s (installed=%v link_healthy=%v)\n", rep.Status, rep.Path, rep.Installed, rep.LinkHealthy)
		if len(rep.DanglingTargets) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "dangling targets: %v\n", rep.DanglingTargets)
		}
		return nil
	},
}

func init() { rootCmd.AddCommand(statusCmd) }

func buildStatusReport(svc *services.Services, e *common.Entry) *statusReport {
	rep := &statusReport{
		Path:            e.Path,
		Status:          e.Status,
		Version:         e.Version,
		Error:           e.Error,
		Modified:        "unknown",
		DanglingTargets: []string{},
	}
	if e.Kind == common.KindSkill {
		var syncErr error
		rep.Sync, syncErr = svc.SyncStatus(e.Path)
		if syncErr != nil {
			rep.SyncError = syncErr.Error()
		}
	}
	if e.Origin != nil {
		rep.Address = &e.Origin.Address
	}
	if rep.Sync != nil {
		switch rep.Sync.Customization {
		case "clean":
			rep.Modified = "clean"
		case "customized":
			rep.Modified = "modified"
		default:
			rep.Modified = "unknown"
		}
	}
	// Archived entries are never installed (models.go), so they have no
	// install state to probe.
	healthy := true
	dangling := []string{}
	if e.Status != common.StatusArchived {
		for _, t := range svc.Installer.Targets(e) {
			switch svc.Installer.State(e, t) {
			case common.InstallInstalled:
				rep.Installed = true
			case common.InstallDangling:
				healthy = false
				dangling = append(dangling, t.Name)
			case common.InstallConflict:
				healthy = false
			}
		}
	}
	rep.LinkHealthy = healthy
	rep.DanglingTargets = dangling
	return rep
}
