package lyria

import "google.golang.org/genai"

// lyricsDraftSchema は LyricsDraft の構造化出力スキーマです。
// ResponseMIMEType "application/json" と併用することで、モデル出力が
// このスキーマに文法レベルで制約されます。
func lyricsDraftSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"title":     {Type: genai.TypeString},
			"theme":     {Type: genai.TypeString},
			"hook":      {Type: genai.TypeString},
			"lyrics":    {Type: genai.TypeString},
			"keywords":  {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
			"mood":      {Type: genai.TypeString},
			"narrative": {Type: genai.TypeString},
		},
		Required: []string{"title", "theme", "hook", "lyrics"},
	}
}

// musicRecipeSchema は MusicRecipe の構造化出力スキーマです。
// Lyrics と AIModels はモデルに生成させず、呼び出し側のコードが付与するため
// 意図的にスキーマへ含めていません。
func musicRecipeSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"title":         {Type: genai.TypeString},
			"theme":         {Type: genai.TypeString},
			"mood":          {Type: genai.TypeString},
			"tempo":         {Type: genai.TypeInteger},
			"key":           {Type: genai.TypeString},
			"vocal_profile": {Type: genai.TypeString},
			"instruments":   {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
			"sections": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"name":             {Type: genai.TypeString},
						"duration_seconds": {Type: genai.TypeInteger},
						"start_seconds":    {Type: genai.TypeInteger},
						"end_seconds":      {Type: genai.TypeInteger},
						"prompt":           {Type: genai.TypeString},
					},
					Required: []string{"name", "duration_seconds", "prompt"},
				},
			},
		},
		Required: []string{"title", "theme", "mood", "tempo", "instruments", "sections"},
	}
}
