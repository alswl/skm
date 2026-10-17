package dal

import (
	"encoding/json"
	"errors"
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
