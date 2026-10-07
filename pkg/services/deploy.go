package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
)

type DeployOptions struct {
	Repo    string
	Into    string
	IntoSet bool
	NoRepo  bool
	Targets []string
	Only    []string
	Force   bool
	DryRun  bool
}

type DeployOperation struct {
	Skill   string  `json:"skill"`
	Target  *string `json:"target"`
	Action  string  `json:"action"`
	Path    string  `json:"path"`
	Status  string  `json:"status"`
	Message string  `json:"message"`
}

type DeployError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type DeployResult struct {
	Repo        string                 `json:"repo"`
	Targets     []string               `json:"targets"`
	Skills      []string               `json:"skills"`
	Clone       string                 `json:"clone"`
	Pull        *string                `json:"pull"`
	Results     []common.InstallReport `json:"results"`
	Success     bool                   `json:"success"`
	Mode        string                 `json:"mode"`
	Destination *string                `json:"destination"`
	DryRun      bool                   `json:"dry_run"`
	Phase       string                 `json:"phase"`
	Operations  []DeployOperation      `json:"operations"`
	Error       *DeployError           `json:"error"`
	Recovery    []string               `json:"recovery"`
}

func NewDeployResult(opts DeployOptions) *DeployResult {
	mode := "repository"
	if opts.NoRepo {
		mode = "direct"
	}
	return &DeployResult{Repo: opts.Repo, Mode: mode, DryRun: opts.DryRun, Phase: "validated", Targets: []string{}, Skills: []string{}, Results: []common.InstallReport{}, Operations: []DeployOperation{}, Recovery: []string{}}
}

func (r *DeployResult) Fail(code string, err error) error {
	r.Success = false
	r.Phase = "failed"
	r.Error = &DeployError{Code: code, Message: err.Error()}
	return err
}

// ValidateDeployOptions can run before config/plugin loading or acquisition.
func ValidateDeployOptions(opts DeployOptions) error {
	if opts.Repo == "" {
		return fmt.Errorf("deploy: --repo is required")
	}
	if len(opts.Targets) == 0 {
		return fmt.Errorf("deploy: --target is required; select target names explicitly")
	}
	for _, n := range opts.Targets {
		if n == "" {
			return fmt.Errorf("deploy: empty target name")
		}
	}
	if opts.NoRepo && (opts.IntoSet || opts.Into != "") {
		return fmt.Errorf("deploy: --into and --no-repo are mutually exclusive")
	}
	if opts.IntoSet && opts.Into == "" {
		return fmt.Errorf("deploy: --into must not be empty")
	}
	return nil
}

// Deploy keeps acquisition separate from durable content: either a new personal
// repository owns it, or explicitly selected targets receive independent copies.
func (s *Services) Deploy(ctx context.Context, opts DeployOptions) (*DeployResult, error) {
	r := NewDeployResult(opts)
	fail := func(code string, err error) (*DeployResult, error) { return r, r.Fail(code, err) }
	if err := ValidateDeployOptions(opts); err != nil {
		return fail("invalid_arguments", err)
	}
	targets, err := s.deployTargets(opts.Targets)
	if err != nil {
		return fail("invalid_targets", err)
	}
	for _, t := range targets {
		r.Targets = append(r.Targets, t.Name)
	}
	destination := ""
	if !opts.NoRepo {
		destination = opts.Into
		if destination == "" {
			destination = "skm"
		}
		destination, err = newDeployDirectory(destination)
		if err != nil {
			return fail("invalid_destination", err)
		}
		r.Destination = &destination
	}
	source, err := s.acquireDeploySource(ctx, opts)
	if err != nil {
		return fail("source_failed", err)
	}
	defer source.cleanup()
	r.Clone = source.clone
	r.Phase = "acquired"
	skills, err := discoverDeploySkills(source.path, opts.Only)
	if err != nil {
		return fail("invalid_skills", err)
	}
	if err = preflightDeploy(source.path, destination, skills, targets, opts.Force); err != nil {
		return fail("preflight_failed", err)
	}
	r.Phase = "preflighted"
	for _, e := range skills {
		r.Skills = append(r.Skills, e.Name)
		if !opts.NoRepo {
			r.Operations = append(r.Operations, DeployOperation{Skill: e.Name, Action: "import", Path: filepath.Join(destination, "skills", source.provider, source.group, e.Name), Status: "not_run"})
		}
	}
	for _, e := range skills {
		for _, t := range targets {
			action := "link"
			if opts.NoRepo {
				action = "copy"
			}
			name := t.Name
			r.Operations = append(r.Operations, DeployOperation{Skill: e.Name, Target: &name, Action: action, Path: filepath.Join(t.Path, e.Name), Status: "not_run"})
		}
	}
	if opts.DryRun {
		for i := range r.Operations {
			r.Operations[i].Status = "planned"
		}
		r.Phase = "planned"
		r.Success = true
		return r, nil
	}
	if err = ctx.Err(); err != nil {
		return fail("cancelled", err)
	}
	if !opts.NoRepo {
		if err = os.Mkdir(destination, 0o755); err != nil {
			return fail("create_failed", err)
		}
		r.Recovery = append(r.Recovery, "Repository content is retained. Complete imports with import --root; retry failed links with install NAME --root and explicit --target. deploy never resumes an existing directory.")
		if _, err = engines.InitializeRepository(destination); err != nil {
			return fail("create_failed", err)
		}
		repo := engines.NewRepository(destination)
		for i, e := range skills {
			op := &r.Operations[i]
			if err = ctx.Err(); err == nil {
				err = engines.ValidateSkillTree(e.Path)
			}
			var imported *engines.RepositoryImportResult
			if err == nil {
				imported, err = repo.ImportStaged(ctx, e.Path, source.provider, source.group, false, source.origin(e.Path))
			}
			if err != nil {
				op.Status = "failed"
				op.Message = err.Error()
				return fail("import_failed", err)
			}
			op.Status = "completed"
			op.Path = imported.Path
			e.Path = imported.Path
			e.Origin = imported.Origin
			e.ProviderID = &imported.Provider
		}
		r.Phase = "imported"
	} else {
		r.Recovery = append(r.Recovery, "Completed target copies are retained. Retry selected skills with --only; use --force only to replace the selected existing slots. Removed upstream skills are not deleted.")
	}
	r.Phase = "installing"
	offset := 0
	if !opts.NoRepo {
		offset = len(skills)
	}
	for _, e := range skills {
		for _, t := range targets {
			op := &r.Operations[offset]
			offset++
			tx := &dal.FileTransaction{}
			changed := false
			if err = ctx.Err(); err == nil {
				if opts.NoRepo {
					changed, err = s.Installer.CopySkill(tx, e, t, opts.Force)
				} else {
					changed, err = s.Installer.Install(tx, e, t, opts.Force)
				}
			}
			if err != nil {
				if rollbackErr := tx.Rollback(); rollbackErr != nil {
					err = fmt.Errorf("%w; rollback: %v", err, rollbackErr)
				}
				op.Status = "failed"
				op.Message = err.Error()
				return fail("install_failed", err)
			}
			tx.Commit()
			op.Status = "completed"
			r.Results = append(r.Results, common.InstallReport{Target: t.Name, Status: common.InstallInstalled, Changed: changed})
		}
	}
	r.Success = true
	r.Phase = "complete"
	r.Recovery = []string{}
	return r, nil
}
