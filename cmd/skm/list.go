package main

import (
	"fmt"
	"sort"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

// listEntry is one catalog row (contract/cli-json.md).
type listEntry struct {
	Name      string           `json:"name"`
	Type      common.EntryKind `json:"type"`
	ModeID    *string          `json:"mode_id"`
	Group     *string          `json:"group"`
	Status    common.Status    `json:"status"`
	Installed bool             `json:"installed"`
	Error     *string          `json:"error"`
}

type listReport struct {
	Root    string      `json:"root"`
	Total   int         `json:"total"`
	Entries []listEntry `json:"entries"`
}

func (e listEntry) ModeIDValue() string {
	if e.ModeID == nil {
		return ""
	}
	return *e.ModeID
}

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List the repository catalog",
	Example: "  skm list --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		rep := buildListReport(svc)
		if flagJSON {
			return printJSON(cmd, rep)
		}
		for _, e := range rep.Entries {
			fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-8s %-8s %s\n", e.Name, e.Type, e.Status, e.ModeIDValue())
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%d entries\n", rep.Total)
		return nil
	},
}

func init() { rootCmd.AddCommand(listCmd) }

func buildListReport(svc *services.Services) *listReport {
	entries := svc.Scan()
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	rep := &listReport{Root: svc.Cfg.Root, Total: len(entries), Entries: []listEntry{}}
	for _, e := range entries {
		rep.Entries = append(rep.Entries, listEntry{
			Name:      e.Name,
			Type:      e.Kind,
			ModeID:    e.ModeID,
			Group:     e.Group,
			Status:    e.Status,
			Installed: entryInstalled(svc, e),
			Error:     e.Error,
		})
	}
	return rep
}

// entryInstalled reports whether the entry has a healthy install in any
// matching target.
func entryInstalled(svc *services.Services, e *common.Entry) bool {
	for _, t := range svc.Installer.Targets(e) {
		if svc.Installer.State(e, t) == common.InstallInstalled {
			return true
		}
	}
	return false
}
