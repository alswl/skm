// Command gendoc generates per-command Markdown documentation for the skm
// CLI from the live Cobra command tree (hack/makefile-go/gen.mk's
// generate-manual target).
package main

import (
	"fmt"
	"os"

	"github.com/alswl/skm/skm/pkg/commands"
	"github.com/spf13/cobra/doc"
)

func main() {
	dir := os.Getenv("CMD_DOCS_DIR")
	if dir == "" {
		dir = "docs"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "gendoc:", err)
		os.Exit(1)
	}
	if err := doc.GenMarkdownTree(commands.RootCmd(), dir); err != nil {
		fmt.Fprintln(os.Stderr, "gendoc:", err)
		os.Exit(1)
	}
}
