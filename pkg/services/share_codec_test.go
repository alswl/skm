package services

import (
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestSharePayloadRoundTripIsCompactAndAlphanumeric(t *testing.T) {
	items := []ShareItem{
		{Source: "https://github.com/acme/skills", Name: "review", Kind: common.KindSkill},
		{Source: "https://github.com/acme/skills", Name: "release", Kind: common.KindCommand},
	}
	payload, err := encodeSharePayload(items)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(payload, shareCodecPrefix))
	require.Regexp(t, `^[A-Za-z0-9]+$`, payload)
	decoded, err := decodeSharePayload(payload)
	require.NoError(t, err)
	require.Equal(t, items, decoded)
}

func TestSharePayloadRejectsNonInlineOrInvalidPrefix(t *testing.T) {
	_, err := decodeSharePayload("file:///tmp/payload")
	require.Error(t, err)
	_, err = decodeSharePayload("skm1znot-base58-!")
	require.Error(t, err)
}
