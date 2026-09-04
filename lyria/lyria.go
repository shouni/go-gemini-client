// Package lyria は、歌詞生成・楽曲設計・Lyria による音声生成を束ねる
// 音楽生成ワークフローを提供します。
//
// 3 段（GenerateLyrics / Compose / GenerateAudio）は個別のメソッドとして公開しており、
// 一括実行の入口は意図的にありません。段の間に挟む品質ゲートは製品ごとに違うため、
// 束ねても呼び出し側で分解し直すことになるからです。
//
// 楽曲の型そのものは music パッケージにあります（MusicRecipe などはその別名です）。
package lyria

import (
	"context"
	"fmt"

	"github.com/shouni/go-gemini-client/callguard"
	"github.com/shouni/go-gemini-client/gemini"
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
func New(aiClient gemini.Generator, promptGen TextPromptGenerator, audioPromptBuilder AudioPromptBuilder, overrides ...Option) (*Workflow, error) {
	opts := applyOptions(overrides...)
	if aiClient == nil {
		return nil, fmt.Errorf("%w: aiClient is required", ErrWorkflowConfig)
	}
	if promptGen == nil {
		return nil, fmt.Errorf("%w: promptGen is required", ErrWorkflowConfig)
	}
	if audioPromptBuilder == nil {
		return nil, fmt.Errorf("%w: audioPromptBuilder is required", ErrWorkflowConfig)
	}
	if opts.geminiModel == "" {
		return nil, fmt.Errorf("%w: GeminiModel is required but not set", ErrWorkflowConfig)
	}
	if opts.lyriaModel == "" {
		return nil, fmt.Errorf("%w: LyriaModel is required but not set", ErrWorkflowConfig)
	}

	// 発射間隔はテキスト（Gemini）と音声（Lyria）で別々に持ちます。別のモデルの
	// 別のクォータなので、片方の混雑でもう片方を絞る理由がありません。
	textGuard := callguard.New(
		callguard.WithRateInterval(opts.textRateInterval),
		callguard.WithExecTimeout(opts.execTimeout),
	)
	audioGuard := callguard.New(
		callguard.WithRateInterval(opts.rateInterval),
		callguard.WithExecTimeout(opts.execTimeout),
	)

	textGenerator := &lyriaTextGenerator{
		aiClient:     aiClient,
		promptGen:    promptGen,
		defaultModel: opts.geminiModel,
		guard:        textGuard,
	}

	return &Workflow{
		lyricist: textGenerator,
		composer: textGenerator,
		audio: &lyriaAudioGenerator{
			aiClient:          aiClient,
			promptBuilder:     audioPromptBuilder,
			guard:             audioGuard,
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
func (w *Workflow) GenerateAudio(ctx context.Context, recipe *MusicRecipe, images []ImagePayload) (*Track, error) {
	return w.audio.GenerateAudio(ctx, recipe, images)
}
