package lyria

import (
	"context"
	"errors"
	"fmt"

	"github.com/shouni/audio/phonetic"
	"github.com/shouni/go-gemini-client/gemini"
	"golang.org/x/time/rate"
)

// Adapter は、歌詞生成・作曲・音声生成を束ねるファサードです。
type Adapter struct {
	lyricist *lyriaTextGenerator
	composer *lyriaTextGenerator
	audio    *lyriaAudioGenerator
}

// NewAdapter は、指定された構成を使用して新しい Adapter を初期化して返します。
func NewAdapter(aiClient gemini.Generator, promptGen TextPromptGenerator, overrides ...Option) (*Adapter, error) {
	opts := applyOptions(overrides...)
	if aiClient == nil {
		return nil, errors.New("aiClient is required")
	}
	if promptGen == nil {
		return nil, errors.New("promptGen is required")
	}
	if opts.geminiModel == "" {
		return nil, errors.New("GeminiModel is required but not set")
	}
	if opts.lyriaModel == "" {
		return nil, errors.New("LyriaModel is required but not set")
	}
	promptBuilder := opts.audioPromptBuilder
	if promptBuilder == nil {
		return nil, errors.New("AudioPromptBuilder is required but not set")
	}

	limiter := rate.NewLimiter(rate.Every(opts.rateInterval), 1)

	converter, err := phonetic.NewConverter()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize phonetic converter: %w", err)
	}

	textGenerator := &lyriaTextGenerator{
		aiClient:     aiClient,
		promptGen:    promptGen,
		defaultModel: opts.geminiModel,
	}

	return &Adapter{
		lyricist: textGenerator,
		composer: textGenerator,
		audio: &lyriaAudioGenerator{
			aiClient:          aiClient,
			promptBuilder:     promptBuilder,
			converter:         converter,
			limiter:           limiter,
			maxConcurrency:    opts.maxConcurrency,
			defaultLyriaModel: opts.lyriaModel,
		},
	}, nil
}

// Run は音楽生成のコアプロセス（作詞〜音声生成）を一括で行います。
func (a *Adapter) Run(ctx context.Context, ai AIModels, input *CollectedContent) (*MusicRecipe, []byte, error) {
	lyrics, err := a.GenerateLyrics(ctx, ai, input)
	if err != nil {
		return nil, nil, err
	}

	recipe, err := a.Compose(ctx, ai, lyrics)
	if err != nil {
		return nil, nil, err
	}

	wav, err := a.GenerateAudio(ctx, recipe, input.Images)
	if err != nil {
		return nil, nil, err
	}

	return recipe, wav, nil
}

// GenerateLyrics builds a lyric draft from collected content.
func (a *Adapter) GenerateLyrics(ctx context.Context, ai AIModels, input *CollectedContent) (*LyricsDraft, error) {
	return a.lyricist.GenerateLyrics(ctx, ai, input)
}

// Compose builds a music recipe from a lyric draft.
func (a *Adapter) Compose(ctx context.Context, ai AIModels, lyrics *LyricsDraft) (*MusicRecipe, error) {
	return a.composer.Compose(ctx, ai, lyrics)
}

// GenerateAudio generates full-song audio from a music recipe.
func (a *Adapter) GenerateAudio(ctx context.Context, recipe *MusicRecipe, images []ImagePayload) ([]byte, error) {
	return a.audio.GenerateAudio(ctx, recipe, images)
}

// GenerateFullAudio generates each section separately and combines the audio.
func (a *Adapter) GenerateFullAudio(ctx context.Context, recipe *MusicRecipe, images []ImagePayload) ([]byte, error) {
	return a.audio.GenerateFullAudio(ctx, recipe, images)
}
