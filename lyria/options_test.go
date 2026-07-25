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
		WithTextRateInterval(100*time.Millisecond),
	)

	assert.Equal(t, "gemini-flash", got.geminiModel)
	assert.Equal(t, "lyria-3", got.lyriaModel)
	assert.Equal(t, 250*time.Millisecond, got.rateInterval)
	assert.Equal(t, 100*time.Millisecond, got.textRateInterval)
}

func TestWithExecTimeout(t *testing.T) {
	t.Run("値が反映されること", func(t *testing.T) {
		got := applyOptions(WithExecTimeout(90 * time.Second))
		assert.Equal(t, 90*time.Second, got.execTimeout)
	})

	t.Run("未指定なら 0 で、doSingleflight 側の既定値にフォールバックすること", func(t *testing.T) {
		got := applyOptions()
		assert.Zero(t, got.execTimeout)
	})
}
