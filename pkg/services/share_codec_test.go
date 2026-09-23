package services

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestSharePayloadRoundTripIsCompactAndURLSafe(t *testing.T) {
	payload := SharePayload{Entries: []ShareEntry{
		{Name: "review", Kind: common.KindSkill, Source: "https://github.com/acme/skills"},
		{Name: "release", Kind: common.KindCommand, Source: "https://github.com/acme/skills"},
	}}
	token, err := encodeSharePayload(payload)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(token, shareCodecPrefix))
	require.Regexp(t, `^[A-Za-z0-9_-]+$`, token)

	decoded, err := decodeSharePayload(token)
	require.NoError(t, err)
	// Canonical order is by kind then name: "command" sorts before "skill".
	require.Equal(t, []ShareEntry{
		{Name: "release", Kind: common.KindCommand, Source: "https://github.com/acme/skills"},
		{Name: "review", Kind: common.KindSkill, Source: "https://github.com/acme/skills"},
	}, decoded.Entries)
}

func TestSharePayloadRejectsMalformedPrefixOrEncoding(t *testing.T) {
	_, err := decodeSharePayload("file:///tmp/payload")
	require.Error(t, err)
	_, err = decodeSharePayload("skm1znot-valid-!")
	require.Error(t, err)
}

func TestSharePayloadRejectsTruncatedToken(t *testing.T) {
	token, err := encodeSharePayload(SharePayload{Entries: []ShareEntry{
		{Name: "a", Kind: common.KindSkill, Source: "https://example.com/a"},
	}})
	require.NoError(t, err)
	decoded, err := decodeSharePayload(token)
	require.NoError(t, err)
	require.NotEmpty(t, decoded.Entries)

	_, err = decodeSharePayload(token[:len(token)-4])
	require.Error(t, err)
}

func TestSharePayloadRejectsDuplicateEntryIdentity(t *testing.T) {
	token, err := encodeSharePayload(SharePayload{Entries: []ShareEntry{
		{Name: "dup", Kind: common.KindSkill, Source: "https://example.com/a"},
		{Name: "dup", Kind: common.KindSkill, Source: "https://example.com/b"},
	}})
	require.NoError(t, err)
	_, err = decodeSharePayload(token)
	require.Error(t, err)
}

func TestSharePayloadRejectsEmptySource(t *testing.T) {
	token, err := encodeSharePayload(SharePayload{Entries: []ShareEntry{
		{Name: "a", Kind: common.KindSkill, Source: "   "},
	}})
	require.NoError(t, err)
	_, err = decodeSharePayload(token)
	require.Error(t, err)
}

func TestSharePayloadRejectsUnsafeEntryName(t *testing.T) {
	token, err := encodeSharePayload(SharePayload{Entries: []ShareEntry{
		{Name: "../escape", Kind: common.KindSkill, Source: "https://example.com/a"},
	}})
	require.NoError(t, err)
	_, err = decodeSharePayload(token)
	require.Error(t, err)
}

// A prior base58 transport took minutes to encode a single megabyte (its
// big-integer conversion is quadratic in input size). This pins the
// linear-time base64 replacement, using a large selection of entries since
// payloads no longer carry file bytes to inflate size with.
func TestSharePayloadEncodesManyEntriesQuickly(t *testing.T) {
	entries := make([]ShareEntry, 0, 500)
	for i := 0; i < 500; i++ {
		entries = append(entries, ShareEntry{
			Name: fmt.Sprintf("%s-%03d", strings.Repeat("x", 40), i), Kind: common.KindSkill,
			Source: fmt.Sprintf("https://github.com/acme/skills-%03d", i),
		})
	}
	token, err := encodeSharePayload(SharePayload{Entries: entries})
	require.NoError(t, err)
	decoded, err := decodeSharePayload(token)
	require.NoError(t, err)
	require.Len(t, decoded.Entries, 500)
}
