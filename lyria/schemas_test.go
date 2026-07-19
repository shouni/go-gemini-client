package lyria

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMusicRecipeSchemaRequiresSectionTimeline は、sections の timeline フィールドが
// 構造化出力スキーマで必須になっていることを保証します。
// Required から漏れるとモデルが start/end_seconds を省略でき、
// 生成レシピの再利用時に timeline 検証で弾かれる不整合が起きます。
func TestMusicRecipeSchemaRequiresSectionTimeline(t *testing.T) {
	schema := musicRecipeSchema()

	sections, ok := schema.Properties["sections"]
	require.True(t, ok, "sections property must exist")
	require.NotNil(t, sections.Items)

	for _, field := range []string{"name", "duration_seconds", "start_seconds", "end_seconds", "prompt"} {
		assert.Contains(t, sections.Items.Required, field)
		_, defined := sections.Items.Properties[field]
		assert.True(t, defined, "required field %q must be defined in properties", field)
	}
}
