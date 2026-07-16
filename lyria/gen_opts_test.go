package lyria

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildJSONGenerateOptionsAppliesSchema(t *testing.T) {
	schema := lyricsDraftSchema()

	got := buildJSONGenerateOptions(nil, schema)

	assert.Equal(t, "application/json", got.ResponseMIMEType)
	assert.Same(t, schema, got.ResponseSchema)
}

func TestBuildAudioGenerateOptionsOmitsNilSeed(t *testing.T) {
	got := buildAudioGenerateOptions(nil)

	assert.Nil(t, got.Seed)
	assert.Empty(t, got.ResponseMIMEType)
}

func TestBuildAudioGenerateOptionsKeepsSeed(t *testing.T) {
	seed := int64(42)

	got := buildAudioGenerateOptions(&seed)

	if assert.NotNil(t, got.Seed) {
		assert.Equal(t, seed, *got.Seed)
	}
	assert.Empty(t, got.ResponseMIMEType)
}
