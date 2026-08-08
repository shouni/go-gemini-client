package lyria

import (
	"github.com/shouni/go-gemini-client/gemini"
	"github.com/shouni/go-gemini-client/music"
)

// 楽曲構成のデータ型は music パッケージへ分離されています。
//
// 型だけが欲しい下流（動画生成・履歴表示など）がワークフロー本体（レート制限や
// singleflight を伴うこのパッケージ）を輸入せずに済むようにするためです。
// ここでの別名は既存の lyria.MusicRecipe 等の表記をそのまま使い続けられるように
// する互換層で、どちらの名前で書いても同じ型です。
type (
	// AIModels selects the text and audio models used by Lyria generation.
	AIModels = music.AIModels
	// LyricsDraft is the structured lyric output used by recipe composition.
	LyricsDraft = music.LyricsDraft
	// MusicRecipe describes the song structure and generation settings.
	MusicRecipe = music.Recipe
	// MusicSection describes one section of a song.
	MusicSection = music.Section
)

// LangJapanese と LangEnglish は MusicRecipe.Lang に指定できる言語コードです。
const (
	LangJapanese = music.LangJapanese
	LangEnglish  = music.LangEnglish
)

// ImagePayload is an optional multimodal image input for audio generation.
//
// gemini.Attachment の別名です。「MIME type とバイト列」という同じ概念を 2 か所で定義すると、
// 呼び出し側が同じ内容を詰め替える羽目になります。別名にしてあるので、既存の
// lyria.ImagePayload{Data: ..., MIMEType: ...} はそのまま書けます。
type ImagePayload = gemini.Attachment

// CollectedContent is the text and image context used for lyrics and music generation.
type CollectedContent struct {
	Prompt string
	Images []ImagePayload
}
