package services

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/fxamacker/cbor/v2"
	"github.com/klauspost/compress/zstd"
	"github.com/mr-tron/base58"
)

const shareCodecPrefix = "skm1z"

var shareAlphabet = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// shareManifest is intentionally compact: sources are stored once and items
// reference them by index. Targets are derived on the receiving machine.
type shareManifest struct {
	Version uint64   `cbor:"0,keyasint"`
	Sources []string `cbor:"1,keyasint"`
	Items   [][]any  `cbor:"2,keyasint"`
}

func encodeSharePayload(items []ShareItem) (string, error) {
	sources := make([]string, 0, len(items))
	sourceIndex := make(map[string]uint64, len(items))
	encodedItems := make([][]any, 0, len(items))
	for _, item := range items {
		source := strings.TrimSpace(item.Source)
		if source == "" {
			return "", fmt.Errorf("share: item %q has an empty source", item.Name)
		}
		idx, ok := sourceIndex[source]
		if !ok {
			idx = uint64(len(sources))
			sourceIndex[source] = idx
			sources = append(sources, source)
		}
		kind := uint64(0)
		if item.Kind == common.KindCommand {
			kind = 1
		}
		encodedItems = append(encodedItems, []any{idx, kind, item.Name})
	}

	manifest := shareManifest{Version: 1, Sources: sources, Items: encodedItems}
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

func decodeSharePayload(payload string) ([]ShareItem, error) {
	payload = strings.TrimSpace(payload)
	if !strings.HasPrefix(payload, shareCodecPrefix) {
		return nil, fmt.Errorf("share: PAYLOAD must start with %q", shareCodecPrefix)
	}
	encoded := strings.TrimPrefix(payload, shareCodecPrefix)
	if encoded == "" || !shareAlphabet.MatchString(encoded) {
		return nil, fmt.Errorf("share: PAYLOAD must contain only letters and digits after %q", shareCodecPrefix)
	}
	compressed, err := base58.Decode(encoded)
	if err != nil {
		return nil, fmt.Errorf("share: decode Base58btc PAYLOAD: %w", err)
	}
	decoder, err := zstd.NewReader(nil, zstd.WithDecoderMaxMemory(64<<20))
	if err != nil {
		return nil, fmt.Errorf("share: create decompressor: %w", err)
	}
	raw, err := decoder.DecodeAll(compressed, nil)
	decoder.Close()
	if err != nil {
		return nil, fmt.Errorf("share: decompress PAYLOAD: %w", err)
	}
	if len(raw) > 16<<20 {
		return nil, fmt.Errorf("share: decoded PAYLOAD is too large")
	}
	var manifest shareManifest
	if err := cbor.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("share: decode CBOR PAYLOAD: %w", err)
	}
	if manifest.Version != 1 {
		return nil, fmt.Errorf("share: unsupported PAYLOAD version %d", manifest.Version)
	}
	if len(manifest.Items) == 0 {
		return nil, fmt.Errorf("share: PAYLOAD contains no items")
	}
	if len(manifest.Items) > 10000 || len(manifest.Sources) > 10000 {
		return nil, fmt.Errorf("share: PAYLOAD contains too many entries")
	}
	items := make([]ShareItem, 0, len(manifest.Items))
	for i, encodedItem := range manifest.Items {
		if len(encodedItem) != 3 {
			return nil, fmt.Errorf("share: item %d has invalid shape", i)
		}
		sourceIndex, ok := encodedItem[0].(uint64)
		if !ok || sourceIndex >= uint64(len(manifest.Sources)) {
			return nil, fmt.Errorf("share: item %d has invalid source index", i)
		}
		kindCode, ok := encodedItem[1].(uint64)
		if !ok || kindCode > 1 {
			return nil, fmt.Errorf("share: item %d has invalid kind", i)
		}
		name, ok := encodedItem[2].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("share: item %d has invalid name", i)
		}
		kind := common.KindSkill
		if kindCode == 1 {
			kind = common.KindCommand
		}
		items = append(items, ShareItem{Source: manifest.Sources[sourceIndex], Name: name, Kind: kind})
	}
	return items, nil
}
