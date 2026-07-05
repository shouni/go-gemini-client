package lyria

import "context"

// MusicWorkflow は音楽生成のコアプロセス（作詞〜音声生成）を一括で行うインターフェースです。
type MusicWorkflow interface {
	// Run はコンテキストを受け取り、最終的な音声バイナリとレシピ（メタデータ）を返します。
	Run(ctx context.Context, ai AIModels, input *CollectedContent) (*MusicRecipe, []byte, error)
}

// Lyricist は歌詞生成を担う役割です。
type Lyricist interface {
	GenerateLyrics(ctx context.Context, ai AIModels, input *CollectedContent) (*LyricsDraft, error)
}

// Composer は楽曲の設計（レシピ構築）を担う役割です。
type Composer interface {
	Compose(ctx context.Context, ai AIModels, lyrics *LyricsDraft) (*MusicRecipe, error)
}

// AudioGenerator は MusicRecipe から音声バイナリを生成します。
type AudioGenerator interface {
	GenerateAudio(ctx context.Context, recipe *MusicRecipe, images []ImagePayload) ([]byte, error)
}

// TextPromptGenerator は歌詞およびレシピ生成のためのプロンプトを構築するインターフェースです。
type TextPromptGenerator interface {
	GenerateLyrics(mode string, input string) (string, error)
	GenerateRecipe(mode string, lyrics *LyricsDraft) (string, error)
}

// AudioPromptBuilder は Lyria の音声生成用プロンプトを構築するインターフェースです。
type AudioPromptBuilder interface {
	BuildFullSong(recipe *MusicRecipe) string
}

// ReadingConverter は Lyria に渡すプロンプトを読み上げ向けの表記に変換します。
type ReadingConverter interface {
	ConvertToReading(input string) string
}

// noopReadingConverter は、WithReadingConverter が指定されなかった場合に使われる
// 何もしないデフォルト実装です。入力をそのまま返します。
// 読み仮名変換が必要な場合は、呼び出し側で ReadingConverter の実装を注入してください。
type noopReadingConverter struct{}

// ConvertToReading は入力をそのまま返します。
func (noopReadingConverter) ConvertToReading(input string) string {
	return input
}
