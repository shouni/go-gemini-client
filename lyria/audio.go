package lyria

import (
	"context"
	"fmt"

	"github.com/shouni/go-gemini-client/callguard"
	"github.com/shouni/go-gemini-client/gemini"
)

// lyriaAudioGenerator は MusicRecipe を Lyria に渡し、音声バイナリを生成します。
type lyriaAudioGenerator struct {
	aiClient          gemini.Generator
	promptBuilder     AudioPromptBuilder
	converter         ReadingConverter
	defaultLyriaModel string
	// guard は発射間隔と 1 回あたりの上限時間です。
	// nil は「制限なし・既定の上限時間」（テストが構造体リテラルで直接構築するため）。
	guard *callguard.Guard
	group callguard.Group
}

// GenerateAudio は MusicRecipe 全体を 1 回の Lyria 呼び出しで音声化します。
func (g *lyriaAudioGenerator) GenerateAudio(ctx context.Context, recipe *MusicRecipe, images []ImagePayload) ([]byte, error) {
	if recipe == nil {
		return nil, fmt.Errorf("%w: music recipe", ErrNilInput)
	}

	targetModel := g.defaultLyriaModel
	if recipe.AudioModel != "" {
		targetModel = recipe.AudioModel
	}

	promptText := g.promptBuilder.BuildFullSong(recipe)
	if recipe.IsJapanese() {
		promptText = g.converter.ConvertToReading(promptText)
	}
	imageHash := calculateImagesHash(images)
	key := callguard.Key("audio-full", targetModel, promptText, callguard.SeedKey(recipe.Seed), imageHash)
	audio, err := callguard.Do(ctx, &g.group, g.guard, key, func(execCtx context.Context) ([]byte, error) {
		resp, err := g.aiClient.GenerateWithAttachments(
			execCtx,
			targetModel,
			promptText,
			images,
			buildAudioGenerateOptions(recipe.Seed),
		)
		if err != nil {
			return nil, fmt.Errorf("lyria generation failed (model: %s): %w", targetModel, err)
		}
		if resp == nil || len(resp.Audios) == 0 {
			return nil, fmt.Errorf("%w (model: %s)", ErrNoAudio, targetModel)
		}

		return resp.Audios[0], nil
	})
	if err != nil {
		return nil, err
	}

	return cloneBytes(audio), nil
}
