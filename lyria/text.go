package lyria

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shouni/go-gemini-client/gemini"
	"golang.org/x/sync/singleflight"
	"google.golang.org/genai"
)

const defaultComposeMode = "default"

// lyriaTextGenerator は Gemini を使った歌詞生成と楽曲レシピ生成をまとめて扱います。
type lyriaTextGenerator struct {
	aiClient     gemini.Generator
	promptGen    TextPromptGenerator
	defaultModel string
	group        singleflight.Group
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

	targetModel := g.defaultModel
	if ai.TextModel != "" {
		targetModel = ai.TextModel
	}

	key := singleflightKey("lyrics", targetModel, promptText)
	lyrics, err := doSingleflight(ctx, &g.group, key, func(execCtx context.Context) (*LyricsDraft, error) {
		parts := []*genai.Part{{Text: promptText}}
		opt := buildJSONGenerateOptions(ai.Seed)
		resp, err := g.aiClient.GenerateWithParts(execCtx, targetModel, parts, opt)
		if err != nil {
			return nil, fmt.Errorf("lyrics generation failed (model: %s): %w", targetModel, err)
		}
		if resp == nil {
			return nil, fmt.Errorf("lyrics response is nil")
		}

		rawLyrics := strings.TrimSpace(resp.Text)
		if rawLyrics == "" {
			return nil, fmt.Errorf("AI returned an empty string for the lyrics")
		}

		var lyrics LyricsDraft
		jsonStr := cleanJSONResponse(rawLyrics)
		if err := json.Unmarshal([]byte(jsonStr), &lyrics); err != nil {
			return nil, fmt.Errorf("failed to unmarshal lyrics json: %w (raw: %s)", err, jsonStr)
		}
		if strings.TrimSpace(lyrics.Lyrics) == "" {
			return nil, fmt.Errorf("lyrics draft is empty")
		}

		return &lyrics, nil
	})
	if err != nil {
		return nil, err
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

	targetModel := g.defaultModel
	if ai.TextModel != "" {
		targetModel = ai.TextModel
	}

	key := singleflightKey("compose", targetModel, promptText)
	recipe, err := doSingleflight(ctx, &g.group, key, func(execCtx context.Context) (*MusicRecipe, error) {
		parts := []*genai.Part{{Text: promptText}}
		opt := buildJSONGenerateOptions(ai.Seed)
		resp, err := g.aiClient.GenerateWithParts(execCtx, targetModel, parts, opt)
		if err != nil {
			return nil, fmt.Errorf("AI generation failed (model: %s): %w", targetModel, err)
		}
		if resp == nil {
			return nil, fmt.Errorf("AI response is nil")
		}

		rawRecipe := strings.TrimSpace(resp.Text)
		if rawRecipe == "" {
			return nil, fmt.Errorf("AI returned an empty string for the recipe")
		}

		jsonStr := cleanJSONResponse(rawRecipe)
		var recipe MusicRecipe
		if err := json.Unmarshal([]byte(jsonStr), &recipe); err != nil {
			return nil, fmt.Errorf("failed to unmarshal recipe json: %w (raw: %s)", err, jsonStr)
		}

		recipe.Lyrics = lyrics
		recipe.AIModels = ai

		return &recipe, nil
	})
	if err != nil {
		return nil, err
	}

	return cloneMusicRecipe(recipe), nil
}
