package lyria

import (
	"context"
	"errors"
	"fmt"

	"github.com/shouni/audio/wav"
	"github.com/shouni/go-gemini-client/gemini"
	"golang.org/x/sync/errgroup"
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
	maxConcurrency    int
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
	responseMIMEType := ""
	imageHash := calculateImagesHash(images)
	key := singleflightKey("audio-full", targetModel, promptText, singleflightSeedKey(recipe.AIModels.Seed), responseMIMEType, imageHash)
	audio, err := doSingleflight(ctx, &g.group, key, func(execCtx context.Context) ([]byte, error) {
		if err := g.limiter.Wait(execCtx); err != nil {
			return nil, err
		}

		resp, err := g.aiClient.GenerateWithParts(
			execCtx,
			targetModel,
			parts,
			buildAudioGenerateOptions(recipe.AIModels.Seed, responseMIMEType),
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

// GenerateFullAudio は MusicRecipe の各セクションを個別に生成し、1 つの WAV に結合します。
func (g *lyriaAudioGenerator) GenerateFullAudio(ctx context.Context, recipe *MusicRecipe, images []ImagePayload) ([]byte, error) {
	if recipe == nil || len(recipe.Sections) == 0 {
		return nil, errors.New("recipe sections are empty")
	}

	wavParts := make([][]byte, len(recipe.Sections))
	eg, egCtx := errgroup.WithContext(ctx)
	if g.maxConcurrency > 0 {
		eg.SetLimit(g.maxConcurrency)
	}

	for i, sec := range recipe.Sections {
		eg.Go(func() error {
			data, err := g.generateAudioSection(egCtx, recipe, sec, images)
			if err != nil {
				return fmt.Errorf("section '%s' generation failed: %w", sec.Name, err)
			}
			wavParts[i] = data
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	combinedWav, err := wav.CombineWavData(wavParts)
	if err != nil {
		return nil, fmt.Errorf("failed to combine wav sections: %w", err)
	}

	return combinedWav, nil
}

// generateAudioSection は指定された 1 セクションを Lyria で音声化します。
func (g *lyriaAudioGenerator) generateAudioSection(ctx context.Context, recipe *MusicRecipe, sec MusicSection, images []ImagePayload) ([]byte, error) {
	if recipe == nil {
		return nil, errors.New("recipe is nil")
	}
	if sec.Prompt == "" {
		return nil, fmt.Errorf("section '%s' prompt is empty", sec.Name)
	}

	targetModel := g.defaultLyriaModel
	if recipe.AudioModel != "" {
		targetModel = recipe.AudioModel
	}

	promptText := g.promptBuilder.BuildSection(recipe, sec)
	parts := g.buildMultiModalParts(promptText, images)
	responseMIMEType := "audio/wav"
	imageHash := calculateImagesHash(images)
	key := singleflightKey("audio-section", targetModel, promptText, singleflightSeedKey(recipe.AIModels.Seed), responseMIMEType, imageHash)
	audio, err := doSingleflight(ctx, &g.group, key, func(execCtx context.Context) ([]byte, error) {
		if err := g.limiter.Wait(execCtx); err != nil {
			return nil, err
		}

		resp, err := g.aiClient.GenerateWithParts(
			execCtx,
			targetModel,
			parts,
			buildAudioGenerateOptions(recipe.AIModels.Seed, responseMIMEType),
		)
		if err != nil {
			return nil, err
		}
		if resp == nil || len(resp.Audios) == 0 {
			return nil, fmt.Errorf("no audio from Lyria for %s", sec.Name)
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
