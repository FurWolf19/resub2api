package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeChatContentParts(t *testing.T) {
	t.Parallel()

	t.Run("string", func(t *testing.T) {
		content, reasoning, isArray, err := NormalizeChatContentParts(json.RawMessage(`"VISIBLE"`))
		require.NoError(t, err)
		require.Equal(t, "VISIBLE", content)
		require.Empty(t, reasoning)
		require.False(t, isArray)
	})

	t.Run("array thinking and text", func(t *testing.T) {
		content, reasoning, isArray, err := NormalizeChatContentParts(json.RawMessage(mistralThinkingContent))
		require.NoError(t, err)
		require.Equal(t, "VISIBLE", content)
		require.Equal(t, "SECRET", reasoning)
		require.True(t, isArray)
	})

	t.Run("unknown part ignored", func(t *testing.T) {
		content, reasoning, isArray, err := NormalizeChatContentParts(json.RawMessage(
			`[{"type":"custom","data":"X"},{"type":"text","text":"VISIBLE"}]`,
		))
		require.NoError(t, err)
		require.Equal(t, "VISIBLE", content)
		require.Empty(t, reasoning)
		require.True(t, isArray)
	})

	t.Run("empty array", func(t *testing.T) {
		content, reasoning, isArray, err := NormalizeChatContentParts(json.RawMessage(`[]`))
		require.NoError(t, err)
		require.Empty(t, content)
		require.Empty(t, reasoning)
		require.True(t, isArray)
	})

	t.Run("null and empty", func(t *testing.T) {
		for _, raw := range []string{"null", ""} {
			content, reasoning, isArray, err := NormalizeChatContentParts(json.RawMessage(raw))
			require.NoError(t, err)
			require.Empty(t, content)
			require.Empty(t, reasoning)
			require.False(t, isArray)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, _, _, err := NormalizeChatContentParts(json.RawMessage(`{"content":`))
		require.Error(t, err)

		_, _, _, err = NormalizeChatContentParts(json.RawMessage(`42`))
		require.Error(t, err)
	})
}

func TestNormalizeChatContentPartsDelta(t *testing.T) {
	t.Parallel()

	content, reasoning, isArray, err := NormalizeChatContentPartsDelta(json.RawMessage(
		`[{"type":"thinking","thinking":[{"type":"text","text":"SEC"}],"closed":true},{"type":"text","text":"a"},{"type":"text","text":"b"}]`,
	))
	require.NoError(t, err)
	require.Equal(t, "ab", content)
	require.Equal(t, "SEC", reasoning)
	require.True(t, isArray)

	content, reasoning, isArray, err = NormalizeChatContentParts(json.RawMessage(
		`[{"type":"text","text":"a"},{"type":"text","text":"b"}]`,
	))
	require.NoError(t, err)
	require.Equal(t, "a\n\nb", content)
	require.Empty(t, reasoning)
	require.True(t, isArray)
}
