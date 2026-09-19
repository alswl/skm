package services

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/fxamacker/cbor/v2"
	"github.com/klauspost/compress/zstd"
	"github.com/mr-tron/base58"
)

const (
	shareCodecPrefix = "skm1z"

	shareManifestVersion = 1

	shareModeManifestContent  = "content"
	shareModeManifestUpstream = "upstream"

	maxShareEntries         = 500
	maxShareFilesPerEntry   = 5000
	maxShareContentFileSize = 4 << 20  // 4MiB per file
	maxShareDecodedSize     = 16 << 20 // 16MiB decoded payload
	maxSharePathLength      = 4096
)

var shareAlphabet = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// shareManifest is the versioned, mode-explicit envelope. Unlike the prior
// source-only manifest, mode is never inferred from which optional fields are
// present (FR-007).
type shareManifest struct {
	Version uint64               `cbor:"0,keyasint"`
	Mode    string               `cbor:"1,keyasint"`
	Entries []shareManifestEntry `cbor:"2,keyasint"`
}

type shareManifestEntry struct {
	Name   string              `cbor:"0,keyasint"`
	Kind   uint64              `cbor:"1,keyasint"`
	Source string              `cbor:"2,keyasint,omitempty"`
	Files  []shareManifestFile `cbor:"3,keyasint,omitempty"`
}

type shareManifestFile struct {
	Path  string `cbor:"0,keyasint"`
	Mode  uint32 `cbor:"1,keyasint"`
	Bytes []byte `cbor:"2,keyasint"`
}

// encodeSharePayload canonicalizes ordering (by kind then name, and files by
// path within each entry) so the same selection always produces the same
// token (data-model.md SharePayload).
func encodeSharePayload(payload SharePayload) (string, error) {
	manifest := shareManifest{Version: shareManifestVersion}
	switch payload.Mode {
	case ShareModeContent:
		manifest.Mode = shareModeManifestContent
		entries := append([]ContentEntry(nil), payload.Content...)
		sort.Slice(entries, func(i, j int) bool {
			return entryLess(entries[i].Kind, entries[i].Name, entries[j].Kind, entries[j].Name)
		})
		for _, e := range entries {
			files := append([]ContentFile(nil), e.Files...)
			sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
			manifestFiles := make([]shareManifestFile, 0, len(files))
			for _, f := range files {
				manifestFiles = append(manifestFiles, shareManifestFile(f))
			}
			manifest.Entries = append(manifest.Entries, shareManifestEntry{
				Name: e.Name, Kind: kindCode(e.Kind), Files: manifestFiles,
			})
		}
	case ShareModeUpstream:
		manifest.Mode = shareModeManifestUpstream
		entries := append([]UpstreamEntry(nil), payload.Upstream...)
		sort.Slice(entries, func(i, j int) bool {
			return entryLess(entries[i].Kind, entries[i].Name, entries[j].Kind, entries[j].Name)
		})
		for _, e := range entries {
			manifest.Entries = append(manifest.Entries, shareManifestEntry{
				Name: e.Name, Kind: kindCode(e.Kind), Source: e.Source,
			})
		}
	default:
		return "", fmt.Errorf("share: unknown payload mode %q", payload.Mode)
	}
	if len(manifest.Entries) == 0 {
		return "", fmt.Errorf("share: payload has no entries")
	}

	encMode, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return "", fmt.Errorf("share: create CBOR encoder: %w", err)
	}
	raw, err := encMode.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("share: encode CBOR: %w", err)
	}
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderCRC(false), zstd.WithEncoderLevel(zstd.SpeedBetterCompression))
	if err != nil {
		return "", fmt.Errorf("share: create compressor: %w", err)
	}
	compressed := encoder.EncodeAll(raw, nil)
	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("share: close compressor: %w", err)
	}
	return shareCodecPrefix + base58.Encode(compressed), nil
}

func entryLess(kindA common.EntryKind, nameA string, kindB common.EntryKind, nameB string) bool {
	if kindA != kindB {
		return kindA < kindB
	}
	return nameA < nameB
}

func kindCode(kind common.EntryKind) uint64 {
	if kind == common.KindCommand {
		return 1
	}
	return 0
}

func decodeKind(code uint64) (common.EntryKind, bool) {
	switch code {
	case 0:
		return common.KindSkill, true
	case 1:
		return common.KindCommand, true
	default:
		return "", false
	}
}

// decodeSharePayload validates the entire payload before any caller can act
// on it: an invalid token, version, mode, identity, or path fails here, before
// any repository write is attempted (FR-010 preflight).
func decodeSharePayload(token string) (SharePayload, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, shareCodecPrefix) {
		return SharePayload{}, fmt.Errorf("share: PAYLOAD must start with %q", shareCodecPrefix)
	}
	encoded := strings.TrimPrefix(token, shareCodecPrefix)
	if encoded == "" || !shareAlphabet.MatchString(encoded) {
		return SharePayload{}, fmt.Errorf("share: PAYLOAD must contain only letters and digits after %q", shareCodecPrefix)
	}
	compressed, err := base58.Decode(encoded)
	if err != nil {
		return SharePayload{}, fmt.Errorf("share: decode Base58btc PAYLOAD: %w", err)
	}
	decoder, err := zstd.NewReader(nil, zstd.WithDecoderMaxMemory(64<<20))
	if err != nil {
		return SharePayload{}, fmt.Errorf("share: create decompressor: %w", err)
	}
	raw, err := decoder.DecodeAll(compressed, nil)
	decoder.Close()
	if err != nil {
		return SharePayload{}, fmt.Errorf("share: decompress PAYLOAD: %w", err)
	}
	if len(raw) > maxShareDecodedSize {
		return SharePayload{}, fmt.Errorf("share: decoded PAYLOAD is too large")
	}
	var manifest shareManifest
	if err := cbor.Unmarshal(raw, &manifest); err != nil {
		return SharePayload{}, fmt.Errorf("share: decode CBOR PAYLOAD: %w", err)
	}
	if manifest.Version != shareManifestVersion {
		return SharePayload{}, fmt.Errorf("share: unsupported PAYLOAD version %d", manifest.Version)
	}
	var mode ShareMode
	switch manifest.Mode {
	case shareModeManifestContent:
		mode = ShareModeContent
	case shareModeManifestUpstream:
		mode = ShareModeUpstream
	default:
		return SharePayload{}, fmt.Errorf("share: unsupported PAYLOAD mode %q", manifest.Mode)
	}
	if len(manifest.Entries) == 0 {
		return SharePayload{}, fmt.Errorf("share: PAYLOAD contains no entries")
	}
	if len(manifest.Entries) > maxShareEntries {
		return SharePayload{}, fmt.Errorf("share: PAYLOAD contains too many entries")
	}

	payload := SharePayload{Mode: mode}
	seen := make(map[string]bool, len(manifest.Entries))
	for i, me := range manifest.Entries {
		kind, ok := decodeKind(me.Kind)
		if !ok {
			return SharePayload{}, fmt.Errorf("share: entry %d has an invalid kind", i)
		}
		name := strings.TrimSpace(me.Name)
		if !validShareEntryName(name) {
			return SharePayload{}, fmt.Errorf("share: entry %d has an invalid name", i)
		}
		key := string(kind) + "\x00" + name
		if seen[key] {
			return SharePayload{}, fmt.Errorf("share: duplicate entry %q (%s)", name, kind)
		}
		seen[key] = true

		switch mode {
		case ShareModeContent:
			if me.Source != "" {
				return SharePayload{}, fmt.Errorf("share: entry %q carries a source in content mode", name)
			}
			ce, err := decodeContentManifestEntry(name, kind, me.Files)
			if err != nil {
				return SharePayload{}, err
			}
			payload.Content = append(payload.Content, ce)
		case ShareModeUpstream:
			if len(me.Files) > 0 {
				return SharePayload{}, fmt.Errorf("share: entry %q carries files in upstream mode", name)
			}
			// Reachability is proven by the apply-time import attempt, not by
			// decode: an unreachable source is a per-item failure, not a
			// malformed payload.
			source := strings.TrimSpace(me.Source)
			if source == "" {
				return SharePayload{}, fmt.Errorf("share: entry %q has an empty upstream source", name)
			}
			payload.Upstream = append(payload.Upstream, UpstreamEntry{Name: name, Kind: kind, Source: source})
		}
	}
	return payload, nil
}

func decodeContentManifestEntry(name string, kind common.EntryKind, files []shareManifestFile) (ContentEntry, error) {
	if len(files) == 0 {
		return ContentEntry{}, fmt.Errorf("share: entry %q has no files", name)
	}
	if len(files) > maxShareFilesPerEntry {
		return ContentEntry{}, fmt.Errorf("share: entry %q has too many files", name)
	}
	marker := kind.MarkerFile()
	hasMarker := false
	seenPaths := make(map[string]bool, len(files))
	out := make([]ContentFile, 0, len(files))
	for _, f := range files {
		if !validShareContentPath(f.Path) {
			return ContentEntry{}, fmt.Errorf("share: entry %q has an unsafe file path %q", name, f.Path)
		}
		if seenPaths[f.Path] {
			return ContentEntry{}, fmt.Errorf("share: entry %q has a duplicate file path %q", name, f.Path)
		}
		seenPaths[f.Path] = true
		if len(f.Bytes) > maxShareContentFileSize {
			return ContentEntry{}, fmt.Errorf("share: entry %q file %q exceeds the size limit", name, f.Path)
		}
		if f.Path == marker {
			hasMarker = true
		}
		out = append(out, ContentFile(f))
	}
	if !hasMarker {
		return ContentEntry{}, fmt.Errorf("share: entry %q is missing its required %s", name, marker)
	}
	return ContentEntry{Name: name, Kind: kind, Files: out}, nil
}

// validShareEntryName rejects anything that is not a single path component,
// mirroring the entry-id safety check ImportStaged already applies.
func validShareEntryName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, "/\\")
}

// validShareContentPath rejects absolute paths, traversal, empty components,
// and `.git`, matching research.md #2's content safety boundary.
func validShareContentPath(p string) bool {
	if p == "" || len(p) > maxSharePathLength {
		return false
	}
	if strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return false
	}
	if path.Clean(p) != p {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." || part == ".git" {
			return false
		}
	}
	return true
}
