package main

import (
	"fmt"
	"sort"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

// inconsistency and nameConflict are verify-report items (contract/cli-json.md).
type inconsistency struct {
	Name  string `json:"name"`
	Issue string `json:"issue"`
}

type nameConflict struct {
	Name  string   `json:"name"`
	Paths []string `json:"paths"`
}

type verifyReport struct {
	Total           int             `json:"total"`
	Active          int             `json:"active"`
	Archived        int             `json:"archived"`
	Errors          int             `json:"errors"`
	Consistent      bool            `json:"consistent"`
	Inconsistencies []inconsistency `json:"inconsistencies"`
	NameConflicts   []nameConflict  `json:"name_conflicts"`
}

var flagNoStrict bool

var verifyCmd = &cobra.Command{
	Use:     "verify repo",
	Short:   "Check whole-repository consistency",
	Example: "  skm verify repo --json\n  skm verify repo --no-strict",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if args[0] != "repo" {
			return common.WithExitCode(fmt.Errorf("verify: unknown target %q (expected \"repo\")", args[0]), common.ExitError)
		}
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		rep := buildVerifyReport(svc)
		if flagJSON {
			if err := printJSON(cmd, rep); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(cmd.OutOrStdout(),
				"total=%d active=%d archived=%d errors=%d consistent=%v name_conflicts=%d\n",
				rep.Total, rep.Active, rep.Archived, rep.Errors, rep.Consistent, len(rep.NameConflicts))
			for _, inc := range rep.Inconsistencies {
				fmt.Fprintf(cmd.OutOrStdout(), "  ! %s: %s\n", inc.Name, inc.Issue)
			}
		}
		if rep.Consistent || flagNoStrict {
			return nil
		}
		// Strict mode: inconsistency is an object problem -> exit 1 (FR-032).
		return common.WithExitCode(
			fmt.Errorf("verify: %d error(s), %d name conflict(s)", rep.Errors, len(rep.NameConflicts)),
			common.ExitObject)
	},
}

func init() {
	verifyCmd.Flags().BoolVar(&flagNoStrict, "no-strict", false, "report inconsistencies without a failing exit")
	rootCmd.AddCommand(verifyCmd)
}

func buildVerifyReport(svc *services.Services) *verifyReport {
	entries := svc.Scan()
	rep := &verifyReport{
		Total:           len(entries),
		Inconsistencies: []inconsistency{},
		NameConflicts:   []nameConflict{},
	}
	seen := map[string][]*common.Entry{}

	for _, e := range entries {
		switch e.Status {
		case common.StatusActive:
			rep.Active++
		case common.StatusArchived:
			rep.Archived++
		case common.StatusError:
			rep.Errors++
			rep.Inconsistencies = append(rep.Inconsistencies, inconsistency{Name: e.Name, Issue: orEmpty(e.Error)})
		}
		seen[e.Name] = append(seen[e.Name], e)
		// Dangling installs are inconsistencies too (UC-13).
		for _, t := range svc.Installer.Targets(e) {
			if svc.Installer.State(e, t) == common.InstallDangling {
				rep.Inconsistencies = append(rep.Inconsistencies, inconsistency{
					Name:  e.Name,
					Issue: fmt.Sprintf("dangling install in target %s", t.Name),
				})
			}
		}
	}

	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if len(seen[n]) > 1 {
			paths := make([]string, 0, len(seen[n]))
			for _, e := range seen[n] {
				paths = append(paths, e.Path)
			}
			rep.NameConflicts = append(rep.NameConflicts, nameConflict{Name: n, Paths: paths})
		}
	}

	rep.Consistent = rep.Errors == 0 && len(rep.Inconsistencies) == 0 && len(rep.NameConflicts) == 0
	return rep
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
