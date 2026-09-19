package services

import (
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestContentSharePayloadRoundTripIsCompactAndAlphanumeric(t *testing.T) {
	payload := SharePayload{
		Mode: ShareModeContent,
		Content: []ContentEntry{
			{Name: "review", Kind: common.KindSkill, Files: []ContentFile{
				{Path: "SKILL.md", Mode: 0o644, Bytes: []byte("---\nname: review\ndescription: d\n---\nbody")},
				{Path: "references/example.md", Mode: 0o644, Bytes: []byte("ref")},
			}},
		},
	}
	token, err := encodeSharePayload(payload)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(token, shareCodecPrefix))
	require.Regexp(t, `^[A-Za-z0-9]+$`, token)

	decoded, err := decodeSharePayload(token)
	require.NoError(t, err)
	require.Equal(t, ShareModeContent, decoded.Mode)
	require.Equal(t, payload.Content, decoded.Content)
}

func TestUpstreamSharePayloadRoundTrip(t *testing.T) {
	payload := SharePayload{
		Mode: ShareModeUpstream,
		Upstream: []UpstreamEntry{
			{Name: "review", Kind: common.KindSkill, Source: "https://github.com/acme/skills"},
		},
	}
	token, err := encodeSharePayload(payload)
	require.NoError(t, err)
	decoded, err := decodeSharePayload(token)
	require.NoError(t, err)
	require.Equal(t, payload, decoded)
}

func TestSharePayloadRejectsMalformedPrefixOrEncoding(t *testing.T) {
	_, err := decodeSharePayload("file:///tmp/payload")
	require.Error(t, err)
	_, err = decodeSharePayload("skm1znot-base58-!")
	require.Error(t, err)
}

func TestSharePayloadRejectsTruncatedToken(t *testing.T) {
	token, err := encodeSharePayload(SharePayload{Mode: ShareModeUpstream, Upstream: []UpstreamEntry{
		{Name: "a", Kind: common.KindSkill, Source: "https://example.com/a"},
	}})
	require.NoError(t, err)
	decoded, err := decodeSharePayload(token)
	require.NoError(t, err)
	require.NotEmpty(t, decoded.Upstream)

	_, err = decodeSharePayload(token[:len(token)-4])
	require.Error(t, err)
}

func TestContentSharePayloadRejectsUnsafePaths(t *testing.T) {
	base := SharePayload{Mode: ShareModeContent, Content: []ContentEntry{{
		Name: "review", Kind: common.KindSkill,
		Files: []ContentFile{{Path: "SKILL.md", Bytes: []byte("body")}},
	}}}

	cases := []string{"/etc/passwd", "../escape", "a/../../b", ".git/config", ""}
	for _, unsafe := range cases {
		p := base
		p.Content = []ContentEntry{{
			Name: "review", Kind: common.KindSkill,
			Files: []ContentFile{
				{Path: "SKILL.md", Bytes: []byte("body")},
				{Path: unsafe, Bytes: []byte("x")},
			},
		}}
		token, err := encodeSharePayload(p)
		require.NoError(t, err)
		_, err = decodeSharePayload(token)
		require.Errorf(t, err, "path %q should be rejected", unsafe)
	}
}

func TestContentSharePayloadRequiresMarkerFile(t *testing.T) {
	token, err := encodeSharePayload(SharePayload{Mode: ShareModeContent, Content: []ContentEntry{{
		Name: "review", Kind: common.KindSkill,
		Files: []ContentFile{{Path: "references/example.md", Bytes: []byte("x")}},
	}}})
	require.NoError(t, err)
	_, err = decodeSharePayload(token)
	require.Error(t, err)
}

func TestSharePayloadRejectsDuplicateEntryIdentity(t *testing.T) {
	token, err := encodeSharePayload(SharePayload{Mode: ShareModeUpstream, Upstream: []UpstreamEntry{
		{Name: "dup", Kind: common.KindSkill, Source: "https://example.com/a"},
		{Name: "dup", Kind: common.KindSkill, Source: "https://example.com/b"},
	}})
	require.NoError(t, err)
	_, err = decodeSharePayload(token)
	require.Error(t, err)
}

func TestUpstreamSharePayloadRejectsEmptySource(t *testing.T) {
	token, err := encodeSharePayload(SharePayload{Mode: ShareModeUpstream, Upstream: []UpstreamEntry{
		{Name: "a", Kind: common.KindSkill, Source: "   "},
	}})
	require.NoError(t, err)
	_, err = decodeSharePayload(token)
	require.Error(t, err)
}
