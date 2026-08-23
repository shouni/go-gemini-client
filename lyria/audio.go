package lyria

import (
	"context"
	"fmt"
	"time"

	"github.com/shouni/go-gemini-client/gemini"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
)

// lyriaAudioGenerator は MusicRecipe を Lyria に渡し、音声バイナリを生成します。
type lyriaAudioGenerator struct {
	aiClient          gemini.Generator
	promptBuilder     AudioPromptBuilder
	converter         ReadingConverter
	defaultLyriaModel string
	limiter           *rate.Limiter // nil はレート制限なし（テストが構造体リテラルで直接構築するため）
	execTimeout       time.Duration
	group             singleflight.Group
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
	key := singleflightKey("audio-full", targetModel, promptText, singleflightSeedKey(recipe.Seed), imageHash)
	audio, err := doSingleflight(ctx, &g.group, key, g.execTimeout, func(execCtx context.Context) ([]byte, error) {
		if g.limiter != nil {
			if err := g.limiter.Wait(execCtx); err != nil {
				return nil, err
			}
		}

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
