package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newMachineDirectoryCommand() *cobra.Command {
	directory := &cobra.Command{Use: "directory", Short: "Manage registered directories"}
	discover := &cobra.Command{
		Use: "discover PARENT", Short: "Discover Git directories", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := servicesFor(cmd)
			if err != nil {
				return err
			}
			items, err := svc.DiscoverDirectoriesReport(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if flagJSON {
				return printJSON(cmd, items)
			}
			for _, item := range items {
				if item.Error != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\terror: %s\n", item.Path, item.Error)
				} else if item.Registered {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\tregistered: %s\n", item.Path, item.Directory)
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), item.Path)
				}
			}
			return nil
		},
	}
	directory.AddCommand(discover)

	var tags []string
	add := &cobra.Command{
		Use: "add ID=PATH [ID=PATH...]", Short: "Register directories", Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entries := make(map[string]string, len(args))
			for _, arg := range args {
				id, path, ok := strings.Cut(arg, "=")
				if !ok || id == "" || path == "" {
					return fmt.Errorf("invalid directory %q; expected ID=PATH", arg)
				}
				if _, exists := entries[id]; exists {
					return fmt.Errorf("directory %q specified twice", id)
				}
				entries[id] = path
			}
			svc, err := servicesFor(cmd)
			if err != nil {
				return err
			}
			if err := svc.RegisterDirectories(cmd.Context(), entries, tags); err != nil {
				return err
			}
			if flagJSON {
				return printJSON(cmd, map[string]bool{"success": true})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "registered %d directories\n", len(entries))
			return nil
		},
	}
	add.Flags().StringSliceVar(&tags, "tag", nil, "tags to add")
	directory.AddCommand(add)

	path := &cobra.Command{
		Use: "path ID PATH", Short: "Set a directory's local path", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := servicesFor(cmd)
			if err != nil {
				return err
			}
			if err := svc.SetDirectoryPath(cmd.Context(), args[0], args[1]); err != nil {
				return err
			}
			if flagJSON {
				return printJSON(cmd, map[string]bool{"success": true})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", args[0])
			return nil
		},
	}
	directory.AddCommand(path)

	var filter []string
	list := &cobra.Command{
		Use: "list", Short: "List registered directories", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := servicesFor(cmd)
			if err != nil {
				return err
			}
			items, err := svc.ListDirectories()
			if err != nil {
				return err
			}
			selected := items[:0]
			for _, item := range items {
				match := len(filter) == 0
				for _, tag := range item.Tags {
					for _, requested := range filter {
						if tag == requested {
							match = true
						}
					}
				}
				if match {
					selected = append(selected, item)
				}
			}
			if flagJSON {
				return printJSON(cmd, selected)
			}
			for _, item := range selected {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", item.ID, item.Path, strings.Join(item.Tags, ","))
			}
			return nil
		},
	}
	list.Flags().StringSliceVar(&filter, "tag", nil, "filter by tag")
	directory.AddCommand(list)

	var ids, addTags, removeTags []string
	tag := &cobra.Command{
		Use: "tag", Short: "Update directory tags", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(ids) == 0 || len(addTags)+len(removeTags) == 0 {
				return fmt.Errorf("select at least one directory and one tag")
			}
			svc, err := servicesFor(cmd)
			if err != nil {
				return err
			}
			if err := svc.TagDirectories(cmd.Context(), ids, addTags, removeTags); err != nil {
				return err
			}
			if flagJSON {
				return printJSON(cmd, map[string]bool{"success": true})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "updated %d directories\n", len(ids))
			return nil
		},
	}
	tag.Flags().StringSliceVar(&ids, "directory", nil, "directory IDs")
	tag.Flags().StringSliceVar(&addTags, "add", nil, "tags to add")
	tag.Flags().StringSliceVar(&removeTags, "remove", nil, "tags to remove")
	directory.AddCommand(tag)

	return directory
}
