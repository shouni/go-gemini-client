package lyria

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shouni/go-gemini-client/gemini"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
	"google.golang.org/genai"
)

const defaultComposeMode = "default"

// lyriaTextGenerator は Gemini を使った歌詞生成と楽曲レシピ生成をまとめて扱います。
type lyriaTextGenerator struct {
	aiClient     gemini.Generator
	promptGen    TextPromptGenerator
	defaultModel string
	limiter      *rate.Limiter // nil の場合はレート制限しない（構造体リテラルでの直接構築との後方互換のため）
	group        singleflight.Group
}

// resolveModel は呼び出しごとのモデル指定があればそれを、なければデフォルトモデルを返します。
func (g *lyriaTextGenerator) resolveModel(override string) string {
	if override != "" {
		return override
	}
	return g.defaultModel
}

// generateJSON は歌詞・レシピ生成で共通の「singleflight → Gemini 呼び出し → JSON デコード」
// フローを実行します。kind はエラーメッセージと singleflight キーの識別子です。
// 戻り値は singleflight で共有されるため、呼び出し側で複製してから返してください。
func generateJSON[T any](ctx context.Context, g *lyriaTextGenerator, kind, model, prompt string, seed *int64, schema *genai.Schema) (*T, error) {
	key := singleflightKey(kind, model, prompt)
	return doSingleflight(ctx, &g.group, key, func(execCtx context.Context) (*T, error) {
		if g.limiter != nil {
			if err := g.limiter.Wait(execCtx); err != nil {
				return nil, err
			}
		}

		parts := []*genai.Part{{Text: prompt}}
		resp, err := g.aiClient.GenerateWithParts(execCtx, model, parts, buildJSONGenerateOptions(seed, schema))
		if err != nil {
			return nil, fmt.Errorf("%s generation failed (model: %s): %w", kind, model, err)
		}
		if resp == nil {
			return nil, fmt.Errorf("%s response is nil", kind)
		}

		raw := strings.TrimSpace(resp.Text)
		if raw == "" {
			return nil, fmt.Errorf("AI returned an empty string for the %s", kind)
		}

		jsonStr := gemini.CleanJSONResponse(raw)
		var out T
		if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %s json: %w (raw: %s)", kind, err, jsonStr)
		}
		return &out, nil
	})
}

// GenerateLyrics は収集済みコンテンツから歌詞ドラフトを生成します。
func (g *lyriaTextGenerator) GenerateLyrics(ctx context.Context, ai AIModels, input *CollectedContent) (*LyricsDraft, error) {
	if input == nil {
		return nil, fmt.Errorf("empty input")
	}

	promptText, err := g.promptGen.GenerateLyrics(ai.LyricsMode, input.Prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to build lyrics prompt: %w", err)
	}

	lyrics, err := generateJSON[LyricsDraft](ctx, g, "lyrics", g.resolveModel(ai.TextModel), promptText, ai.Seed, lyricsDraftSchema())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(lyrics.Lyrics) == "" {
		return nil, fmt.Errorf("lyrics draft is empty")
	}

	return cloneLyricsDraft(lyrics), nil
}

// Compose は歌詞ドラフトから楽曲レシピを生成します。
func (g *lyriaTextGenerator) Compose(ctx context.Context, ai AIModels, lyrics *LyricsDraft) (*MusicRecipe, error) {
	if lyrics == nil {
		return nil, fmt.Errorf("lyrics cannot be nil")
	}

	targetMode := ai.ComposeMode
	if targetMode == "" {
		targetMode = defaultComposeMode
	}

	promptText, err := g.promptGen.GenerateRecipe(targetMode, lyrics)
	if err != nil {
		return nil, fmt.Errorf("failed to build prompt (mode: %s): %w", targetMode, err)
	}

	shared, err := generateJSON[MusicRecipe](ctx, g, "compose", g.resolveModel(ai.TextModel), promptText, ai.Seed, musicRecipeSchema())
	if err != nil {
		return nil, err
	}

	// 呼び出し元固有の情報は共有結果を複製してから付与する。
	recipe := cloneMusicRecipe(shared)
	recipe.Lyrics = cloneLyricsDraft(lyrics)
	recipe.AIModels = ai
	if ai.Seed != nil {
		seed := *ai.Seed
		recipe.Seed = &seed
	}
	return recipe, nil
}
