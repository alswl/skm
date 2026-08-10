package commands

import (
	"fmt"
	"os"

	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:     "init [DIRECTORY]",
	Short:   "Initialize an empty skills repository",
	Example: "  skm init\n  skm init ./my-skills",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("init: determine current directory: %w", err)
		}
		if len(args) == 1 {
			path = args[0]
		}
		root, err := services.InitializeRepository(path)
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, map[string]string{"root": root, "next": "skm import PATH --root " + root})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "initialized skills repository at %s\nnext: skm import PATH --root %s\n", root, root)
		return nil
	},
}

func init() { rootCmd.AddCommand(initCmd) }
