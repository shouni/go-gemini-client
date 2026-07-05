package lyria

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestApplyOptionsAppliesFunctionalOptions(t *testing.T) {
	t.Parallel()

	got := applyOptions(
		WithGeminiModel("gemini-flash"),
		WithLyriaModel("lyria-3"),
		WithRateInterval(250*time.Millisecond),
	)

	assert.Equal(t, "gemini-flash", got.geminiModel)
	assert.Equal(t, "lyria-3", got.lyriaModel)
	assert.Equal(t, 250*time.Millisecond, got.rateInterval)
}
