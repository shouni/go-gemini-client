package lyria

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMusicSectionMarshalIncludesTimelineBounds(t *testing.T) {
	t.Parallel()

	section := MusicSection{
		Name:         "Intro",
		Duration:     12,
		StartSeconds: 0,
		EndSeconds:   12,
		Prompt:       "soft opening",
	}

	raw, err := json.Marshal(section)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, float64(0), got["start_seconds"])
	assert.Equal(t, float64(12), got["end_seconds"])
}
