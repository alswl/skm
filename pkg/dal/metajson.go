package dal

import (
	"encoding/json"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
)

// metaFileName is the origin/provenance file inside an entry directory.
const metaFileName = "meta.json"

// MetaFilePath returns the absolute meta.json path for an entry directory.
func MetaFilePath(entryDir string) string {
	return filepath.Join(entryDir, metaFileName)
}

// ReadMeta reads <entry>/meta.json into an Origin. os.ErrNotExist is returned
// when the entry has no meta.json (no origin).
func ReadMeta(entryDir string) (*common.Origin, error) {
	data, err := os.ReadFile(MetaFilePath(entryDir))
	if err != nil {
		return nil, err
	}
	var o common.Origin
	if err := json.Unmarshal(data, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// WriteMeta writes the origin to <entry>/meta.json.
func WriteMeta(entryDir string, origin *common.Origin) error {
	if origin == nil {
		return RemoveMeta(entryDir)
	}
	data, err := json.MarshalIndent(origin, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(MetaFilePath(entryDir), data, 0o644)
}

// RemoveMeta deletes <entry>/meta.json if present.
func RemoveMeta(entryDir string) error {
	path := MetaFilePath(entryDir)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ReadSkillMeta accepts both source-only legacy metadata and the final v3
// format. A caller may read a legacy origin but must migrate it before a v3
// write operation; it cannot infer a historical revision from this reader.
func ReadSkillMeta(entryDir string) (*common.SkillMeta, error) {
	data, err := readRegularMeta(entryDir)
	if err != nil {
		return nil, err
	}
	var meta common.SkillMeta
	if err = json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if err = validateSkillMeta(meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func WriteSkillMeta(entryDir string, meta common.SkillMeta) error {
	if meta.SchemaVersion != 3 {
		return fmt.Errorf("invalid skill metadata schema %d", meta.SchemaVersion)
	}
	if err := validateSkillMeta(meta); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeRegularMeta(entryDir, data)
}

func validateSkillMeta(meta common.SkillMeta) error {
	if meta.SchemaVersion == 0 { // source-only legacy meta.json
		if meta.Address == "" {
			return fmt.Errorf("invalid legacy skill metadata: missing source address")
		}
		return nil
	}
	if meta.SchemaVersion != 3 {
		return fmt.Errorf("unsupported skill metadata schema %d", meta.SchemaVersion)
	}
	if meta.Address == "" {
		return fmt.Errorf("invalid skill metadata: missing source address")
	}
	if meta.Path != "" && (!filepath.IsLocal(meta.Path) || meta.Path == ".") {
		return fmt.Errorf("invalid skill metadata path %q", meta.Path)
	}
	if meta.Active != "upstream" && meta.Active != "fork" {
		return fmt.Errorf("invalid skill metadata active choice %q", meta.Active)
	}
	switch meta.RevisionKind {
	case "git":
		if meta.RepositoryAddress == "" || !validGitObjectID(meta.Revision) {
			return fmt.Errorf("invalid skill metadata: Git source needs repository and full commit")
		}
	case "snapshot":
		if meta.Revision != "" {
			return fmt.Errorf("invalid skill metadata: snapshot cannot claim a Git revision")
		}
	default:
		return fmt.Errorf("invalid skill metadata revision kind %q", meta.RevisionKind)
	}
	return nil
}

func ReadForkMeta(entryDir string) (*common.ForkMeta, error) {
	data, err := readRegularMeta(entryDir)
	if err != nil {
		return nil, err
	}
	var meta common.ForkMeta
	if err = json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if err = validateForkMeta(meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func WriteForkMeta(entryDir string, meta common.ForkMeta) error {
	if err := validateForkMeta(meta); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeRegularMeta(entryDir, data)
}

func validateForkMeta(meta common.ForkMeta) error {
	if meta.SchemaVersion != 3 {
		return fmt.Errorf("unsupported fork metadata schema %d", meta.SchemaVersion)
	}
	if meta.BaseRevision != "" && !validGitObjectID(meta.BaseRevision) {
		return fmt.Errorf("invalid fork base revision")
	}
	return nil
}

func validGitObjectID(id string) bool {
	if len(id) != 40 && len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func readRegularMeta(entryDir string) ([]byte, error) {
	path := MetaFilePath(entryDir)
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("invalid metadata file: %s", path)
	}
	return os.ReadFile(path)
}

func writeRegularMeta(entryDir string, data []byte) error {
	path := MetaFilePath(entryDir)
	if st, err := os.Lstat(path); err == nil {
		if !st.Mode().IsRegular() {
			return fmt.Errorf("invalid metadata file: %s", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
