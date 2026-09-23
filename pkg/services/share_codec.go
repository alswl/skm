package services

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/fxamacker/cbor/v2"
	"github.com/klauspost/compress/zstd"
)

const (
	shareCodecPrefix = "skm1z"

	shareManifestVersion = 1

	maxShareEntries = 500
)

// shareTokenEncoding is base64 (not base58): base58's big-integer conversion
// is quadratic in input size and was measured taking minutes to encode a
// single megabyte. base64 is linear and stays copy/paste-safe inside the
// single-quoted apply command.
var shareTokenEncoding = base64.RawURLEncoding

var shareAlphabet = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// shareManifest is the versioned envelope (data-model.md SharePayload).
type shareManifest struct {
	Version uint64               `cbor:"0,keyasint"`
	Entries []shareManifestEntry `cbor:"1,keyasint"`
}

type shareManifestEntry struct {
	Name   string `cbor:"0,keyasint"`
	Kind   uint64 `cbor:"1,keyasint"`
	Source string `cbor:"2,keyasint"`
}

// encodeSharePayload canonicalizes ordering (by kind then name) so the same
// selection always produces the same token.
func encodeSharePayload(payload SharePayload) (string, error) {
	entries := append([]ShareEntry(nil), payload.Entries...)
	sort.Slice(entries, func(i, j int) bool {
		return entryLess(entries[i].Kind, entries[i].Name, entries[j].Kind, entries[j].Name)
	})
	if len(entries) == 0 {
		return "", fmt.Errorf("share: payload has no entries")
	}

	manifest := shareManifest{Version: shareManifestVersion}
	for _, e := range entries {
		manifest.Entries = append(manifest.Entries, shareManifestEntry{
			Name: e.Name, Kind: kindCode(e.Kind), Source: e.Source,
		})
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
	return shareCodecPrefix + shareTokenEncoding.EncodeToString(compressed), nil
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
// on it: an invalid token, version, identity, or source fails here, before
// any repository write is attempted (preflight).
func decodeSharePayload(token string) (SharePayload, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, shareCodecPrefix) {
		return SharePayload{}, fmt.Errorf("share: PAYLOAD must start with %q", shareCodecPrefix)
	}
	encoded := strings.TrimPrefix(token, shareCodecPrefix)
	if encoded == "" || !shareAlphabet.MatchString(encoded) {
		return SharePayload{}, fmt.Errorf("share: PAYLOAD contains invalid characters after %q", shareCodecPrefix)
	}
	compressed, err := shareTokenEncoding.DecodeString(encoded)
	if err != nil {
		return SharePayload{}, fmt.Errorf("share: decode PAYLOAD: %w", err)
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
	if len(raw) > 4<<20 {
		return SharePayload{}, fmt.Errorf("share: decoded PAYLOAD is too large")
	}
	var manifest shareManifest
	if err := cbor.Unmarshal(raw, &manifest); err != nil {
		return SharePayload{}, fmt.Errorf("share: decode CBOR PAYLOAD: %w", err)
	}
	if manifest.Version != shareManifestVersion {
		return SharePayload{}, fmt.Errorf("share: unsupported PAYLOAD version %d", manifest.Version)
	}
	if len(manifest.Entries) == 0 {
		return SharePayload{}, fmt.Errorf("share: PAYLOAD contains no entries")
	}
	if len(manifest.Entries) > maxShareEntries {
		return SharePayload{}, fmt.Errorf("share: PAYLOAD contains too many entries")
	}

	payload := SharePayload{}
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

		source := strings.TrimSpace(me.Source)
		if source == "" {
			return SharePayload{}, fmt.Errorf("share: entry %q has an empty source", name)
		}
		payload.Entries = append(payload.Entries, ShareEntry{Name: name, Kind: kind, Source: source})
	}
	return payload, nil
}

// validShareEntryName rejects anything that is not a single path component,
// mirroring the entry-id safety check ImportStaged already applies.
func validShareEntryName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, "/\\")
}
