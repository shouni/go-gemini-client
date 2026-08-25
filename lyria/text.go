package lyria

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shouni/go-gemini-client/callguard"
	"github.com/shouni/go-gemini-client/gemini"
)

const defaultComposeMode = "default"

// lyriaTextGenerator は Gemini を使った歌詞生成と楽曲レシピ生成をまとめて扱います。
type lyriaTextGenerator struct {
	aiClient     gemini.Generator
	promptGen    TextPromptGenerator
	defaultModel string
	// guard は nil で「制限なし・既定の上限時間」を意味します。
	guard *callguard.Guard
	group callguard.Group
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
func generateJSON[T any](ctx context.Context, g *lyriaTextGenerator, kind, model, prompt string, seed *int64, schema *gemini.Schema) (*T, error) {
	// seed は生成結果を変えるため、必ずキーに含める。含め忘れると同一プロンプトで
	// seed 違いの同時呼び出しが 1 回の生成結果を共有してしまう。
	key := callguard.Key(kind, model, prompt, callguard.SeedKey(seed))
	return callguard.Do(ctx, &g.group, g.guard, key, func(execCtx context.Context) (*T, error) {
		resp, err := g.aiClient.GenerateWithAttachments(execCtx, model, prompt, nil, buildJSONGenerateOptions(seed, schema))
		if err != nil {
			return nil, fmt.Errorf("%s generation failed (model: %s): %w", kind, model, err)
		}
		if resp == nil {
			return nil, fmt.Errorf("%w: %s response is nil", ErrInvalidResponse, kind)
		}

		raw := strings.TrimSpace(resp.Text)
		if raw == "" {
			return nil, fmt.Errorf("%w: AI returned an empty string for the %s", ErrInvalidResponse, kind)
		}

		jsonStr := gemini.CleanJSONResponse(raw)
		var out T
		if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
			// 生出力の全文はログを肥大化させるため、診断に足りる先頭だけを残す。
			return nil, fmt.Errorf("%w: failed to unmarshal %s json: %w (raw: %s)",
				ErrInvalidResponse, kind, err, truncateForError(jsonStr))
		}
		return &out, nil
	})
}

// GenerateLyrics は収集済みコンテンツから歌詞ドラフトを生成します。
func (g *lyriaTextGenerator) GenerateLyrics(ctx context.Context, ai AIModels, input *CollectedContent) (*LyricsDraft, error) {
	if input == nil {
		return nil, fmt.Errorf("%w: collected content", ErrNilInput)
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
		return nil, ErrEmptyLyrics
	}

	return lyrics.Clone(), nil
}

// Compose は歌詞ドラフトから楽曲レシピを生成します。
func (g *lyriaTextGenerator) Compose(ctx context.Context, ai AIModels, lyrics *LyricsDraft) (*MusicRecipe, error) {
	if lyrics == nil {
		return nil, fmt.Errorf("%w: lyrics draft", ErrNilInput)
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
	recipe := shared.Clone()
	recipe.Lyrics = lyrics.Clone()
	recipe.AIModels = ai
	if ai.Seed != nil {
		seed := *ai.Seed
		recipe.Seed = &seed
	}
	return recipe, nil
}
