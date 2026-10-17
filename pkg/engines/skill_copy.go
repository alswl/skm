package engines

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

func ValidateSkillName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("invalid skill name %q", name)
	}
	return nil
}

// ValidateSkillTree is read-only, including during dry-run. A copied skill must
// not depend on links outside its own durable content or contain special files.
func ValidateSkillTree(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill contains symlink %q", path)
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("skill contains special file %q", path)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, readErr := io.Copy(io.Discard, f)
		closeErr := f.Close()
		if readErr != nil {
			return readErr
		}
		return closeErr
	})
}

// prepareImport is shared by repository import and independent target copies.
func (r *Repository) prepareImport(source, name string) (string, error) {
	tmp, err := r.stageCopy(source)
	if err != nil {
		return "", err
	}
	if err = normalizeStagedSkill(tmp, name); err == nil {
		if _, statErr := os.Stat(filepath.Join(tmp, "SKILL.md")); statErr == nil {
			err = os.RemoveAll(filepath.Join(tmp, "meta.json"))
		}
	}
	if err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	return tmp, nil
}

// CopySkill stages a full, normalized copy, never a link to the source.
func CopySkill(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget, force bool) (bool, error) {
	if err := ValidateSkillName(entry.Name); err != nil {
		return false, err
	}
	if err := ValidateSkillTree(entry.Path); err != nil {
		return false, err
	}
	repo := NewRepository("")
	tmp, err := repo.prepareImport(entry.Path, entry.Name)
	if err != nil {
		return false, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err = ValidateSkillTree(tmp); err != nil {
		return false, err
	}
	dest := filepath.Join(target.Path, entry.Name)
	if _, err = os.Lstat(dest); err == nil && !force {
		return false, common.WithExitCode(fmt.Errorf("copy: %q already exists; use --force", dest), common.ExitObject)
	} else if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if !force {
		if err = tx.ReserveDirectory(dest); err != nil {
			return false, err
		}
	}
	if err = tx.MoveStage(tmp, dest); err != nil {
		return false, err
	}
	return true, nil
}
