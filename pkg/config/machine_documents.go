package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"gopkg.in/yaml.v3"
)

const machineVersion = 1

// InstallationsFile is the versioned installation declaration at the library root.
const InstallationsFile = "installations.yaml"

var machineName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// InstallationsDocument stores the versioned desired installations.
type InstallationsDocument struct {
	Version     int                         `yaml:"version" json:"version"`
	Directories map[string]common.Directory `yaml:"directories" json:"directories"`
	Installs    []common.Installation       `yaml:"installs" json:"installs"`
}

// DirectoryPaths stores the machine-specific paths of registered directories.
type DirectoryPaths struct {
	Version int               `yaml:"version" json:"version"`
	Paths   map[string]string `yaml:"paths" json:"paths"`
}

func readMachineYAML(path string, out any) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple YAML documents")
		}
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return true, nil
}

// LoadInstallations loads and validates the versioned declaration.
func LoadInstallations(path string) (InstallationsDocument, error) {
	var doc InstallationsDocument
	found, err := readMachineYAML(path, &doc)
	if err != nil {
		return doc, err
	}
	if !found {
		doc.Version = machineVersion
		return doc, nil
	}
	if doc.Version != machineVersion {
		return doc, fmt.Errorf("unsupported installations version %d", doc.Version)
	}
	for id, directory := range doc.Directories {
		if !machineName.MatchString(id) {
			return doc, fmt.Errorf("invalid directory ID %q", id)
		}
		tags := make([]string, 0, len(directory.Tags))
		seen := make(map[string]bool)
		for _, tag := range directory.Tags {
			if !machineName.MatchString(tag) {
				return doc, fmt.Errorf("invalid tag %q", tag)
			}
			if !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
		directory.Tags = tags
		doc.Directories[id] = directory
	}
	installs := make([]common.Installation, 0, len(doc.Installs))
	seenInstalls := make(map[common.Installation]bool)
	for _, install := range doc.Installs {
		clean := filepath.Clean(install.Skill)
		if install.Skill == "" || filepath.IsAbs(install.Skill) || clean != install.Skill ||
			clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return doc, fmt.Errorf("invalid skill path %q", install.Skill)
		}
		if install.Target == "" {
			return doc, fmt.Errorf("missing target for %q", install.Skill)
		}
		switch install.Scope.Kind {
		case "user":
			if install.Scope.Name != "" {
				return doc, fmt.Errorf("user scope must not have a name")
			}
		case "directory":
			if install.Target != "codex" && install.Target != "claude" {
				return doc, fmt.Errorf("unsupported directory target %q", install.Target)
			}
			if _, ok := doc.Directories[install.Scope.Name]; !ok {
				return doc, fmt.Errorf("unknown directory %q", install.Scope.Name)
			}
		case "tag":
			if install.Target != "codex" && install.Target != "claude" {
				return doc, fmt.Errorf("unsupported directory target %q", install.Target)
			}
			if !machineName.MatchString(install.Scope.Name) {
				return doc, fmt.Errorf("invalid tag %q", install.Scope.Name)
			}
		default:
			return doc, fmt.Errorf("invalid scope %q", install.Scope.Kind)
		}
		if !seenInstalls[install] {
			seenInstalls[install] = true
			installs = append(installs, install)
		}
	}
	doc.Installs = installs
	return doc, nil
}

// LoadDirectoryPaths loads local paths and checks their directory references.
func LoadDirectoryPaths(path string, doc InstallationsDocument) (DirectoryPaths, error) {
	var paths DirectoryPaths
	found, err := readMachineYAML(path, &paths)
	if err != nil {
		return paths, err
	}
	if !found {
		paths.Version = machineVersion
		return paths, nil
	}
	if paths.Version != machineVersion {
		return paths, fmt.Errorf("unsupported directory paths version %d", paths.Version)
	}
	seen := make(map[string]string)
	for id, local := range paths.Paths {
		if _, ok := doc.Directories[id]; !ok {
			return paths, fmt.Errorf("unknown directory %q", id)
		}
		if !filepath.IsAbs(local) || filepath.Clean(local) != local {
			return paths, fmt.Errorf("invalid local path %q", local)
		}
		physical, err := filepath.EvalSymlinks(local)
		if err != nil {
			if !os.IsNotExist(err) {
				return paths, fmt.Errorf("inspect %q: %w", local, err)
			}
			physical, err = machineCanonicalMissing(local)
			if err != nil {
				return paths, fmt.Errorf("inspect %q: %w", local, err)
			}
		}
		if previous, ok := seen[physical]; ok {
			return paths, fmt.Errorf("directories %q and %q share path %q", previous, id, physical)
		}
		seen[physical] = id
		paths.Paths[id] = physical
	}
	return paths, nil
}

func machineCanonicalMissing(path string) (string, error) {
	ancestor := path
	for {
		physical, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			relative, err := filepath.Rel(ancestor, path)
			if err != nil {
				return "", err
			}
			return filepath.Join(physical, relative), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", err
		}
		ancestor = parent
	}
}

// DirectoryPathsFile returns the local state path for a canonical library root.
func DirectoryPathsFile(configDir, root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	return filepath.Join(configDir, "directory-paths", hex.EncodeToString(sum[:])+".yaml"), nil
}

// WriteYAMLAtomic replaces a YAML document atomically.
func WriteYAMLAtomic(path string, value any, mode os.FileMode) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".installations-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer func() { _ = os.Remove(tmp) }()
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
