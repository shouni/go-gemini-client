package lyria

import "context"

// MusicRunner は音楽生成のコアプロセス（作詞〜音声生成）を一括で行うインターフェースです。
type MusicRunner interface {
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
	GenerateFullAudio(ctx context.Context, recipe *MusicRecipe, images []ImagePayload) ([]byte, error)
}

// TextPromptGenerator は歌詞およびレシピ生成のためのプロンプトを構築するインターフェースです。
type TextPromptGenerator interface {
	GenerateLyrics(mode string, input string) (string, error)
	GenerateRecipe(mode string, lyrics *LyricsDraft) (string, error)
}

// PromptGenerator は TextPromptGenerator の互換性維持のためのエイリアスです。
type PromptGenerator = TextPromptGenerator

// AudioPromptBuilder は Lyria の音声生成用プロンプトを構築するインターフェースです。
type AudioPromptBuilder interface {
	BuildFullSong(recipe *MusicRecipe) string
	BuildSection(recipe *MusicRecipe, section MusicSection) string
}
