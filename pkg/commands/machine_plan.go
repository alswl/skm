package commands

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

type machinePlanDocument struct {
	Version     int                           `json:"version"`
	Action      string                        `json:"action"`
	Request     json.RawMessage               `json:"request"`
	Digest      string                        `json:"digest"`
	Items       []common.InstallationPlanItem `json:"items"`
	Diagnostics []common.InstallationPlanItem `json:"diagnostics"`
	Summary     map[string]int                `json:"summary"`
}

type machineLinkRequest struct {
	Directory string `json:"directory,omitempty"`
	Target    string `json:"target"`
	Path      string `json:"path"`
}

func makeMachinePlan(action, digest string, request any, items []common.InstallationPlanItem) (*machinePlanDocument, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	doc := &machinePlanDocument{Version: 1, Action: action, Request: data, Digest: digest, Items: []common.InstallationPlanItem{}, Diagnostics: []common.InstallationPlanItem{}, Summary: map[string]int{}}
	seen := map[string]bool{}
	for _, item := range items {
		key := item.Path + "\x00" + item.Skill
		if item.Path == "" {
			key = item.Directory + "\x00" + item.Target + "\x00" + item.Skill
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		doc.Items = append(doc.Items, item)
		doc.Summary[item.Status]++
		switch item.Status {
		case "conflict", "missing_path", "error", "unignored", "changed", "recovery":
			doc.Diagnostics = append(doc.Diagnostics, item)
		}
	}
	doc.Summary["total"] = len(doc.Items)
	return doc, nil
}

func decodeMachineJSON(data []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func readMachinePlan(data []byte) (*machinePlanDocument, error) {
	var plan machinePlanDocument
	if err := decodeMachineJSON(data, &plan); err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	if plan.Version != 1 || len(plan.Request) == 0 || len(plan.Digest) != 64 {
		return nil, fmt.Errorf("invalid installation plan version, request or digest")
	}
	if _, err := hex.DecodeString(plan.Digest); err != nil {
		return nil, fmt.Errorf("invalid installation plan digest: %w", err)
	}
	return &plan, nil
}

func newMachinePlanCommand() *cobra.Command {
	var unlinkPath, dropSkill string
	var capturePaths, planTargets, planTags, planDirectories, addSkills []string
	var dropUser, allowUnignored bool
	plan := &cobra.Command{
		Use: "plan", Short: "Preview installation changes", Args: cobra.NoArgs,
		Long: `Preview installations required by installations.yaml, or a selected declaration change.

Use --add with skills, targets, and exactly one scope type (--user, --directory,
or --tag). Use --capture for existing matching links, --drop for an exact
declaration, or --unlink for one uncovered matching link. Capture and unlink
require a target and an explicit user or directory scope. Multiple --capture,
--add, --target and same-type scope flags can be supplied where applicable.

Save --json output to a file and review it before machine apply. Use
--allow-unignored only when project links outside Git ignore rules are accepted.
Unlink requires verified exclusive rename support on the selected filesystem;
on Linux, unrecognized filesystem types are blocked during preview.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			actions := 0
			if len(capturePaths) != 0 {
				actions++
			}
			if dropSkill != "" {
				actions++
			}
			if unlinkPath != "" {
				actions++
			}
			if len(addSkills) != 0 {
				actions++
			}
			if actions > 1 || (actions != 0 && len(planTargets) == 0) {
				return fmt.Errorf("select at most one action and provide its target")
			}
			svc, err := servicesFor(cmd)
			if err != nil {
				return err
			}
			if actions == 0 {
				if len(planTargets) != 0 {
					return fmt.Errorf("--target requires an explicit action")
				}
				result, err := svc.PlanMachineRestoreScoped(services.MachineRestoreRequest{User: dropUser, Directories: planDirectories, Tags: planTags, AllowUnignored: allowUnignored})
				if err != nil {
					return err
				}
				var items []common.InstallationPlanItem
				for _, batch := range result.Plans {
					items = append(items, batch.Items...)
				}
				for _, observation := range result.Observations {
					status := observation.Status
					if status == "installed" {
						status = "undeclared"
					}
					items = append(items, common.InstallationPlanItem{Directory: observation.Directory, Target: observation.Target, Skill: observation.Skill, Path: observation.Path, Status: status, Error: observation.Error})
				}
				doc, err := makeMachinePlan("restore", result.Digest, result.Request, items)
				if err != nil {
					return err
				}
				if flagJSON {
					return printJSON(cmd, doc)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%d declarations, %d installation locations\n", len(result.Plans), doc.Summary["total"])
				for _, status := range []string{"install", "installed", "undeclared", "unignored", "conflict", "missing_path", "error", "recovery"} {
					if doc.Summary[status] > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "%s: %d\n", status, doc.Summary[status])
					}
				}
				return nil
			}
			if len(addSkills) != 0 {
				scopes := 0
				if dropUser {
					scopes++
				}
				if len(planTags) != 0 {
					scopes++
				}
				if len(planDirectories) != 0 {
					scopes++
				}
				if scopes != 1 {
					return fmt.Errorf("add requires exactly one scope type")
				}
				result, err := svc.PlanMachineBatch(services.MachineBatchRequest{Skills: addSkills, Targets: planTargets, Tags: planTags, Directories: planDirectories, User: dropUser, AllowUnignored: allowUnignored})
				if err != nil {
					return err
				}
				if flagJSON {
					doc, err := makeMachinePlan("add", result.Digest, result.Request, result.Items)
					if err != nil {
						return err
					}
					return printJSON(cmd, doc)
				}
				counts := make(map[string]int)
				for _, item := range result.Items {
					counts[item.Status]++
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%d directories, %d installation locations\n", len(result.Directories), len(result.Items))
				for _, status := range []string{"install", "installed", "unignored", "conflict", "missing_path", "error"} {
					if counts[status] > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "%s: %d\n", status, counts[status])
					}
				}
				return nil
			}
			if len(planTargets) != 1 || len(planTags) > 1 || len(planDirectories) > 1 || allowUnignored {
				return fmt.Errorf("capture and drop require one target and at most one tag")
			}
			captureTarget := planTargets[0]
			captureDirectory := ""
			if len(planDirectories) == 1 {
				captureDirectory = planDirectories[0]
			}
			dropTag := ""
			if len(planTags) == 1 {
				dropTag = planTags[0]
			}
			if len(capturePaths) != 0 {
				if (captureDirectory == "") == !dropUser || dropTag != "" {
					return fmt.Errorf("capture requires exactly one of --directory or --user")
				}
				result, err := svc.PlanMachineCaptures(services.MachineCapturesRequest{Directory: captureDirectory, Target: captureTarget, Paths: capturePaths})
				if err != nil {
					return err
				}
				doc, err := makeMachinePlan("capture", result.Digest, result.Request, result.Items)
				if err != nil {
					return err
				}
				return printJSON(cmd, doc)
			}
			if unlinkPath != "" {
				if (captureDirectory == "") == !dropUser || dropTag != "" {
					return fmt.Errorf("unlink requires exactly one of --directory or --user")
				}
				result, err := svc.PlanMachineUnlink(captureDirectory, captureTarget, unlinkPath)
				if err != nil {
					if errors.Is(err, services.ErrMachineUnlinkCovered) || errors.Is(err, services.ErrMachineUnlinkCoverageUnknown) || errors.Is(err, common.ErrMachineRenameUnavailable) {
						request := machineLinkRequest{Directory: captureDirectory, Target: captureTarget, Path: unlinkPath}
						data, marshalErr := json.Marshal(request)
						if marshalErr != nil {
							return marshalErr
						}
						sum := sha256.Sum256(append([]byte("blocked-unlink\x00"), data...))
						doc, planErr := makeMachinePlan("unlink", hex.EncodeToString(sum[:]), request, []common.InstallationPlanItem{{Directory: captureDirectory, Target: captureTarget, Path: unlinkPath, Status: "conflict", Error: err.Error()}})
						if planErr != nil {
							return planErr
						}
						return printJSON(cmd, doc)
					}
					return err
				}
				doc, err := makeMachinePlan("unlink", result.Digest, machineLinkRequest{Directory: result.Directory, Target: result.Target, Path: result.Path}, []common.InstallationPlanItem{{Directory: result.Directory, Target: result.Target, Skill: result.Skill, Path: result.Path, Status: "unlink"}})
				if err != nil {
					return err
				}
				return printJSON(cmd, doc)
			}
			var scope common.InstallationScope
			switch {
			case captureDirectory != "" && dropTag == "" && !dropUser:
				scope = common.InstallationScope{Kind: "directory", Name: captureDirectory}
			case captureDirectory == "" && dropTag != "" && !dropUser:
				scope = common.InstallationScope{Kind: "tag", Name: dropTag}
			case captureDirectory == "" && dropTag == "" && dropUser:
				scope = common.InstallationScope{Kind: "user"}
			default:
				return fmt.Errorf("select exactly one scope")
			}
			install := common.Installation{Skill: dropSkill, Target: captureTarget, Scope: scope}
			result, err := svc.PlanMachineDrop(install)
			if err != nil {
				entry, resolveErr := svc.ResolveEntry(dropSkill)
				if resolveErr != nil {
					return resolveErr
				}
				if entry == nil {
					return err
				}
				install.Skill = svc.MachineSkillReference(entry)
				result, err = svc.PlanMachineDrop(install)
			}
			if err != nil {
				return err
			}
			doc, err := makeMachinePlan("drop", result.Digest, result.Install, []common.InstallationPlanItem{{Target: result.Install.Target, Skill: result.Install.Skill, Status: "drop"}})
			if err != nil {
				return err
			}
			return printJSON(cmd, doc)
		},
	}
	plan.Flags().StringSliceVar(&capturePaths, "capture", nil, "existing link paths")
	plan.Flags().StringVar(&unlinkPath, "unlink", "", "existing link to remove")
	plan.Flags().StringSliceVar(&planTargets, "target", nil, "target names")
	plan.Flags().StringSliceVar(&planDirectories, "directory", nil, "registered directory IDs")
	plan.Flags().StringVar(&dropSkill, "drop", "", "skill declaration to remove")
	plan.Flags().StringSliceVar(&planTags, "tag", nil, "tag scopes")
	plan.Flags().StringSliceVar(&addSkills, "add", nil, "skills to declare and install")
	plan.Flags().BoolVar(&allowUnignored, "allow-unignored", false, "accept unignored project links")
	plan.Flags().BoolVar(&dropUser, "user", false, "user scope")
	return plan

}

func newMachineApplyCommand() *cobra.Command {
	var planFile string
	apply := &cobra.Command{
		Use: "apply", Short: "Apply a reviewed installation plan", Args: cobra.NoArgs,
		Long: `Recheck the selected request against current declarations and disk state,
then apply the reviewed plan. A stale plan must be generated again. Individual
installation conflicts are reported without overwriting existing objects.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if planFile == "" {
				return fmt.Errorf("--plan is required")
			}
			data, err := os.ReadFile(planFile)
			if err != nil {
				return err
			}
			doc, err := readMachinePlan(data)
			if err != nil {
				return common.WithExitCode(err, common.ExitError)
			}
			svc, err := servicesFor(cmd)
			if err != nil {
				return err
			}
			var single common.InstallationPlanItem
			switch doc.Action {
			case "unlink":
				var request machineLinkRequest
				if err := decodeMachineJSON(doc.Request, &request); err != nil {
					return common.WithExitCode(err, common.ExitError)
				}
				selected := services.MachineUnlinkPlan{Action: "unlink", Version: doc.Version, Directory: request.Directory, Target: request.Target, Path: request.Path, Digest: doc.Digest}
				single = common.InstallationPlanItem{Directory: request.Directory, Target: request.Target, Path: request.Path, Status: "unlinked"}
				err = svc.ApplyMachineUnlink(cmd.Context(), selected)
			case "restore":
				var request services.MachineRestoreRequest
				if err := decodeMachineJSON(doc.Request, &request); err != nil {
					return common.WithExitCode(err, common.ExitError)
				}
				selected := services.MachineRestorePlan{Action: "restore", Version: doc.Version, Request: request, Digest: doc.Digest}
				result, err := svc.ApplyMachineRestore(cmd.Context(), selected)
				if err != nil {
					return common.WithExitCode(err, common.ExitObject)
				}
				if flagJSON {
					if err := printJSON(cmd, result); err != nil {
						return err
					}
				} else {
					for _, item := range result.Items {
						fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s", item.Directory, item.Target, item.Skill, item.Status)
						if item.Error != "" {
							fmt.Fprintf(cmd.OutOrStdout(), "\t%s", item.Error)
						}
						fmt.Fprintln(cmd.OutOrStdout())
					}
				}
				for _, item := range result.Items {
					if item.Status != "installed" {
						return common.WithExitCode(fmt.Errorf("one or more installations were not applied"), common.ExitObject)
					}
				}
				return nil
			case "add":
				var request services.MachineBatchRequest
				if err := decodeMachineJSON(doc.Request, &request); err != nil {
					return common.WithExitCode(err, common.ExitError)
				}
				selected := services.MachineBatchPlan{Action: "add", Version: doc.Version, Request: request, Digest: doc.Digest}
				result, err := svc.ApplyMachineBatch(cmd.Context(), selected)
				if err != nil {
					return common.WithExitCode(err, common.ExitObject)
				}
				if flagJSON {
					if err := printJSON(cmd, result); err != nil {
						return err
					}
				} else {
					for _, item := range result.Items {
						fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s", item.Directory, item.Target, item.Skill, item.Status)
						if item.Error != "" {
							fmt.Fprintf(cmd.OutOrStdout(), "\t%s", item.Error)
						}
						fmt.Fprintln(cmd.OutOrStdout())
					}
				}
				for _, item := range result.Items {
					if item.Status != "installed" {
						return common.WithExitCode(fmt.Errorf("one or more installations were not applied"), common.ExitObject)
					}
				}
				return nil
			case "capture":
				var request services.MachineCapturesRequest
				if err := decodeMachineJSON(doc.Request, &request); err != nil {
					return common.WithExitCode(err, common.ExitError)
				}
				selected := services.MachineCapturesPlan{Action: "capture", Version: doc.Version, Request: request, Digest: doc.Digest}
				result, err := svc.ApplyMachineCaptures(cmd.Context(), selected)
				if err != nil {
					return common.WithExitCode(err, common.ExitObject)
				}
				if flagJSON {
					if err := printJSON(cmd, result); err != nil {
						return err
					}
				} else {
					for _, item := range result.Items {
						fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s", item.Target, item.Path, item.Status)
						if item.Error != "" {
							fmt.Fprintf(cmd.OutOrStdout(), "\t%s", item.Error)
						}
						fmt.Fprintln(cmd.OutOrStdout())
					}
				}
				for _, item := range result.Items {
					if item.Status != "captured" {
						return common.WithExitCode(fmt.Errorf("one or more captures failed"), common.ExitObject)
					}
				}
				return nil
			case "drop":
				var install common.Installation
				if err := decodeMachineJSON(doc.Request, &install); err != nil {
					return common.WithExitCode(err, common.ExitError)
				}
				selected := services.MachineDropPlan{Action: "drop", Version: doc.Version, Install: install, Digest: doc.Digest}
				single = common.InstallationPlanItem{Target: install.Target, Skill: install.Skill, Status: "dropped"}
				err = svc.ApplyMachineDrop(cmd.Context(), selected)
			default:
				return common.WithExitCode(fmt.Errorf("unsupported plan action %q", doc.Action), common.ExitError)
			}
			if err != nil {
				single.Status, single.Error = "error", err.Error()
			}
			if flagJSON {
				if printErr := printJSON(cmd, services.MachineBatchResult{Items: []common.InstallationPlanItem{single}}); printErr != nil {
					return printErr
				}
			} else if err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "plan applied")
			}
			return common.WithExitCode(err, common.ExitObject)
		},
	}
	apply.Flags().StringVar(&planFile, "plan", "", "reviewed plan file")
	return apply
}
