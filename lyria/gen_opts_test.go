package lyria

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildAudioGenerateOptionsOmitsNilSeed(t *testing.T) {
	got := buildAudioGenerateOptions(nil, "audio/wav")

	assert.Nil(t, got.Seed)
	assert.Equal(t, "audio/wav", got.ResponseMIMEType)
}

func TestBuildAudioGenerateOptionsKeepsSeed(t *testing.T) {
	seed := int64(42)

	got := buildAudioGenerateOptions(&seed, "audio/wav")

	if assert.NotNil(t, got.Seed) {
		assert.Equal(t, seed, *got.Seed)
	}
	assert.Equal(t, "audio/wav", got.ResponseMIMEType)
}
