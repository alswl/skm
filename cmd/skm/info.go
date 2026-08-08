package main

import (
	"fmt"
	"os"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

// infoReport is the entry detail report (contract/cli-json.md).
type infoReport struct {
	Type        common.EntryKind  `json:"type"`
	Path        string            `json:"path"`
	Version     *string           `json:"version"`
	Frontmatter map[string]string `json:"frontmatter"`
	Files       []string          `json:"files"`
}

var infoCmd = &cobra.Command{
	Use:     "info NAME",
	Short:   "Show entry metadata, files and marker frontmatter",
	Example: "  skm info review --json",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		entry := svc.FindEntry(args[0])
		if entry == nil {
			return common.WithExitCode(fmt.Errorf("info: entry %q not found", args[0]), common.ExitObject)
		}
		rep, err := buildInfoReport(svc, entry)
		if err != nil {
			return common.WithExitCode(err, common.ExitObject)
		}
		if flagJSON {
			return printJSON(cmd, rep)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n", rep.Path, rep.Type)
		for _, f := range rep.Files {
			fmt.Fprintln(cmd.OutOrStdout(), "  "+f)
		}
		return nil
	},
}

func init() { rootCmd.AddCommand(infoCmd) }

func buildInfoReport(svc *services.Services, e *common.Entry) (*infoReport, error) {
	rep := &infoReport{
		Type:        e.Kind,
		Path:        e.Path,
		Version:     e.Version,
		Frontmatter: map[string]string{},
		Files:       svc.EntryFiles(e.Path),
	}
	if rep.Files == nil {
		rep.Files = []string{}
	}
	data, err := os.ReadFile(e.MarkerPath())
	if err != nil {
		return nil, fmt.Errorf("info: read marker: %w", err)
	}
	fm, _, err := svc.EntryFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("info: parse frontmatter: %w", err)
	}
	rep.Frontmatter["name"] = fm.Name
	rep.Frontmatter["description"] = fm.Description
	if fm.Version != nil && *fm.Version != "" {
		rep.Frontmatter["version"] = *fm.Version
	}
	return rep, nil
}
