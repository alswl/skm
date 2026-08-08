package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/alswl/skm/skm/pkg/tui"
	"github.com/spf13/cobra"
)

// Global flags, shared by every command (exit-codes.md). They are defined
// once here and inherited via Cobra's persistent flags.
var (
	flagRoot   string
	flagConfig string
	flagJSON   bool
	flagTiming bool
	flagDryRun bool
	flagForce  bool
)

var rootCmd = &cobra.Command{
	Use:   "skm",
	Short: "Manage a local repository of AI coding skills and commands",
	Long: `skm manages a repository of AI coding skills and commands: browse,
search, install, import, update, archive, convert and replicate them.

Run with no subcommand to open the interactive TUI; run a known subcommand
(e.g. skm list --json) for scriptable CLI output.`,
	// A known subcommand dispatches to its command; only when no subcommand is
	// matched does this fall through to the TUI (FR-001). The TUI is therefore
	// never initialized for a CLI invocation.
	// Silence cobra's default error/usage printing so stdout stays JSON-clean;
	// the top-level Execute() owns diagnostics on stderr (FR-030).
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI(cmd.Context())
	},
}

// RootCmd returns the root Cobra command, for tooling that walks the command
// tree without executing it (e.g. cmd/gendoc's Markdown generation).
func RootCmd() *cobra.Command {
	return rootCmd
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rootCmd.SetContext(ctx)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "skm:", err)
		return common.ExitCodeOf(err, common.ExitError)
	}
	return common.ExitOK
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVar(&flagRoot, "root", "", "repository root (or a skills/ directory); auto-discovered when omitted")
	pf.StringVar(&flagConfig, "config", "", "config directory holding targets.json (default ~/.config/skm)")
	pf.BoolVar(&flagJSON, "json", false, "emit a JSON report on stdout")
	pf.BoolVar(&flagTiming, "timing", false, "write timing info to stderr only")
	pf.BoolVar(&flagDryRun, "dry-run", false, "perform no writes; report intended actions")
	pf.BoolVar(&flagForce, "force", false, "explicitly authorize overwrite/destructive actions")
}

// servicesFor builds a Services instance from the global flags, resolving the
// repository root and config. Command files call this from their RunE.
func servicesFor(cmd *cobra.Command) (*services.Services, error) {
	cfg, err := config.Load(flagRoot, flagConfig)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	return services.New(cfg, common.NewLogger(flagTiming))
}

// deployServicesFor builds Services for the deploy command without requiring a
// local repository root (the deploy source is on the target machine).
func deployServicesFor(cmd *cobra.Command) (*services.Services, error) {
	cfg := config.LoadForDeploy(flagConfig)
	return services.New(cfg, common.NewLogger(flagTiming))
}

// runTUI starts the Bubble Tea interface. It is only reachable when no known
// subcommand was provided.
func runTUI(ctx context.Context) error {
	cfg, err := config.Load(flagRoot, flagConfig)
	if err != nil {
		return common.WithExitCode(err, common.ExitError)
	}
	svc, err := services.New(cfg, common.NewLogger(flagTiming))
	if err != nil {
		return common.WithExitCode(err, common.ExitError)
	}
	return tui.Run(ctx, svc)
}
