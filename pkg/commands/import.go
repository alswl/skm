package commands

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	isatty "github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var importFlags struct {
	provider string
	kind     string
}

var importCmd = &cobra.Command{
	Use:   "import SOURCE",
	Short: "Import a skill or command from a local path or provider address",
	Long: `Import a skill or command from a local path or provider address.

SOURCE may be "-" to read newline-separated sources from stdin, so a list can
be piped in. Blank lines and lines starting with # are skipped.`,
	Example: "  skm import ./my-skill --kind skill --force\n" +
		"  skm import git@github.com:org/repo.git --json\n" +
		"  cat sources.txt | skm import -",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		if args[0] != "-" {
			result, err := importOne(cmd, svc, args[0])
			if err != nil {
				return err
			}
			if flagJSON {
				return printJSON(cmd, result)
			}
			printImported(cmd, result)
			return nil
		}

		sources, err := readSourceList(cmd.InOrStdin())
		if err != nil {
			return err
		}
		// Successes are reported even when a later source fails, so stdout
		// still describes what actually landed on disk.
		results := make([]*services.ImportResult, 0, len(sources))
		var failure error
		for _, source := range sources {
			result, err := importOne(cmd, svc, source)
			if err != nil {
				failure = fmt.Errorf("%s: %w", source, err)
				break
			}
			results = append(results, result)
			if !flagJSON {
				printImported(cmd, result)
			}
		}
		if flagJSON {
			if err := printJSON(cmd, results); err != nil {
				return err
			}
		}
		return failure
	},
}

func importOne(cmd *cobra.Command, svc *services.Services, source string) (*services.ImportResult, error) {
	return svc.Import(cmd.Context(), source, services.ImportOptions{
		Provider: importFlags.provider,
		Kind:     importFlags.kind,
		Force:    flagForce,
		DryRun:   flagDryRun,
	})
}

func printImported(cmd *cobra.Command, r *services.ImportResult) {
	fmt.Fprintf(cmd.OutOrStdout(), "imported %s (%s) via %s -> %s\n", r.Name, r.Type, r.Provider, r.Path)
}

// readSourceList reads newline-separated sources, skipping blanks and #
// comments. Reading from an interactive terminal is refused rather than
// silently waiting for input the caller did not mean to type.
func readSourceList(in io.Reader) ([]string, error) {
	if f, ok := in.(*os.File); ok && isatty.IsTerminal(f.Fd()) {
		return nil, common.WithExitCode(
			fmt.Errorf(`import -: stdin is a terminal; pipe a list of sources (e.g. "cat sources.txt | skm import -")`),
			common.ExitError)
	}
	var sources []string
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sources = append(sources, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, common.WithExitCode(fmt.Errorf("import -: reading stdin: %w", err), common.ExitError)
	}
	if len(sources) == 0 {
		return nil, common.WithExitCode(fmt.Errorf("import -: stdin contained no sources"), common.ExitError)
	}
	return sources, nil
}

func init() {
	importCmd.Flags().StringVar(&importFlags.provider, "provider", "", "provider id for remote imports")
	importCmd.Flags().StringVar(&importFlags.kind, "kind", "auto", "kind hint: auto|skill|command")
	rootCmd.AddCommand(importCmd)
}
