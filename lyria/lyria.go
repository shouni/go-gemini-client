package lyria

import (
	"context"
	"errors"

	"github.com/shouni/go-gemini-client/gemini"
	"golang.org/x/time/rate"
)

// Workflow がパッケージ公開インターフェースを満たすことをコンパイル時に保証します。
// これらのアサーションがないと、メソッドシグネチャがドリフトしても
// 下流の利用側がビルドされるまで気付けません。
var (
	_ Lyricist       = (*Workflow)(nil)
	_ Composer       = (*Workflow)(nil)
	_ AudioGenerator = (*Workflow)(nil)
)

// Workflow は、歌詞生成・作曲・音声生成を束ねるファサードです。
type Workflow struct {
	lyricist Lyricist
	composer Composer
	audio    AudioGenerator
}

// New は、指定された構成を使用して新しい Workflow を初期化して返します。
func New(aiClient gemini.MultimodalGenerator, promptGen TextPromptGenerator, audioPromptBuilder AudioPromptBuilder, overrides ...Option) (*Workflow, error) {
	opts := applyOptions(overrides...)
	if aiClient == nil {
		return nil, errors.New("aiClient is required")
	}
	if promptGen == nil {
		return nil, errors.New("promptGen is required")
	}
	if audioPromptBuilder == nil {
		return nil, errors.New("audioPromptBuilder is required")
	}
	if opts.geminiModel == "" {
		return nil, errors.New("GeminiModel is required but not set")
	}
	if opts.lyriaModel == "" {
		return nil, errors.New("LyriaModel is required but not set")
	}

	limiter := rate.NewLimiter(rate.Every(opts.rateInterval), 1)
	textLimiter := rate.NewLimiter(rate.Every(opts.textRateInterval), 1)

	converter := opts.readingConverter
	if converter == nil {
		converter = noopReadingConverter{}
	}

	textGenerator := &lyriaTextGenerator{
		aiClient:     aiClient,
		promptGen:    promptGen,
		defaultModel: opts.geminiModel,
		limiter:      textLimiter,
		execTimeout:  opts.execTimeout,
	}

	return &Workflow{
		lyricist: textGenerator,
		composer: textGenerator,
		audio: &lyriaAudioGenerator{
			aiClient:          aiClient,
			promptBuilder:     audioPromptBuilder,
			converter:         converter,
			limiter:           limiter,
			execTimeout:       opts.execTimeout,
			defaultLyriaModel: opts.lyriaModel,
		},
	}, nil
}

// GenerateLyrics builds a lyric draft from collected content.
func (w *Workflow) GenerateLyrics(ctx context.Context, ai AIModels, input *CollectedContent) (*LyricsDraft, error) {
	return w.lyricist.GenerateLyrics(ctx, ai, input)
}

// Compose builds a music recipe from a lyric draft.
func (w *Workflow) Compose(ctx context.Context, ai AIModels, lyrics *LyricsDraft) (*MusicRecipe, error) {
	return w.composer.Compose(ctx, ai, lyrics)
}

// GenerateAudio generates full-song audio from a music recipe.
func (w *Workflow) GenerateAudio(ctx context.Context, recipe *MusicRecipe, images []ImagePayload) ([]byte, error) {
	return w.audio.GenerateAudio(ctx, recipe, images)
}
