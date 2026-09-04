package lyria

import "context"

// Lyricist は歌詞生成を担う役割です。
type Lyricist interface {
	GenerateLyrics(ctx context.Context, ai AIModels, input *CollectedContent) (*LyricsDraft, error)
}

// Composer は楽曲の設計（レシピ構築）を担う役割です。
type Composer interface {
	Compose(ctx context.Context, ai AIModels, lyrics *LyricsDraft) (*MusicRecipe, error)
}

// AudioGenerator は MusicRecipe から音声を生成します。
type AudioGenerator interface {
	GenerateAudio(ctx context.Context, recipe *MusicRecipe, images []ImagePayload) (*Track, error)
}

// TextPromptGenerator は歌詞およびレシピ生成のためのプロンプトを構築するインターフェースです。
type TextPromptGenerator interface {
	GenerateLyrics(mode string, input string) (string, error)
	GenerateRecipe(mode string, lyrics *LyricsDraft) (string, error)
}

// AudioPromptBuilder は Lyria の音声生成用プロンプトを構築するインターフェースです。
//
// 読み仮名変換のような表記の加工も、実装側の仕事です。このパッケージには以前
// WithReadingConverter があり、組み上がったプロンプト全文を変換していましたが、全文変換は
// Title や Theme のような「歌わせない文脈情報」まで読み表記へ潰し、発音上の利点なしに
// 意味だけを失わせます。どの部分が歌詞行なのかを知っているのはプロンプトを組んだ側だけ
// なので、変換の適用範囲もそこでしか決められません。
type AudioPromptBuilder interface {
	BuildFullSong(recipe *MusicRecipe) string
}
