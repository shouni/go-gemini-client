// Package lyria は、歌詞生成・楽曲設計・Lyriaによる音声生成を束ねる
// 音楽生成ワークフローを提供します。
package lyria

import (
	"context"
	"fmt"

	"github.com/shouni/go-gemini-client/gemini"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
	"google.golang.org/genai"
)

// lyriaAudioGenerator は MusicRecipe を Lyria に渡し、音声バイナリを生成します。
type lyriaAudioGenerator struct {
	aiClient          gemini.Generator
	promptBuilder     AudioPromptBuilder
	converter         ReadingConverter
	defaultLyriaModel string
	limiter           *rate.Limiter
	group             singleflight.Group
}

// GenerateAudio は MusicRecipe 全体を 1 回の Lyria 呼び出しで音声化します。
func (g *lyriaAudioGenerator) GenerateAudio(ctx context.Context, recipe *MusicRecipe, images []ImagePayload) ([]byte, error) {
	if recipe == nil {
		return nil, fmt.Errorf("recipe cannot be nil")
	}

	targetModel := g.defaultLyriaModel
	if recipe.AudioModel != "" {
		targetModel = recipe.AudioModel
	}

	promptText := g.promptBuilder.BuildFullSong(recipe)
	parts := g.buildMultiModalParts(promptText, images)
	imageHash := calculateImagesHash(images)
	key := singleflightKey("audio-full", targetModel, promptText, singleflightSeedKey(recipe.Seed), imageHash)
	audio, err := doSingleflight(ctx, &g.group, key, func(execCtx context.Context) ([]byte, error) {
		if err := g.limiter.Wait(execCtx); err != nil {
			return nil, err
		}

		resp, err := g.aiClient.GenerateWithParts(
			execCtx,
			targetModel,
			parts,
			buildAudioGenerateOptions(recipe.Seed),
		)
		if err != nil {
			return nil, fmt.Errorf("lyria generation failed (model: %s): %w", targetModel, err)
		}
		if resp == nil || len(resp.Audios) == 0 {
			return nil, fmt.Errorf("no audio data received from Lyria")
		}

		return resp.Audios[0], nil
	})
	if err != nil {
		return nil, err
	}

	return cloneBytes(audio), nil
}

// buildMultiModalParts はプロンプトと画像を Lyria 入力用の Part スライスにまとめます。
func (g *lyriaAudioGenerator) buildMultiModalParts(prompt string, images []ImagePayload) []*genai.Part {
	parts := make([]*genai.Part, 0, len(images)+1)
	safePrompt := g.converter.ConvertToReading(prompt)
	parts = append(parts, &genai.Part{Text: safePrompt})

	for _, img := range images {
		if len(img.Data) == 0 {
			continue
		}
		parts = append(parts, &genai.Part{
			InlineData: &genai.Blob{
				MIMEType: img.MIMEType,
				Data:     img.Data,
			},
		})
	}
	return parts
}
