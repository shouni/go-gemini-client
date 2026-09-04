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
// 型そのものの説明は music 側にあります。ここへ書き写すと必ずずれます。
type (
	// AIModels は music.AIModels の別名です。
	AIModels = music.AIModels
	// LyricsDraft は music.LyricsDraft の別名です。
	LyricsDraft = music.LyricsDraft
	// MusicRecipe は music.Recipe の別名です。
	MusicRecipe = music.Recipe
	// MusicSection は music.Section の別名です。
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

// Track は 1 回の音声生成の結果です。
//
// 音声バイト列だけを返していた頃は、同じレスポンスに載っている MIME type とテキストが
// 捨てられていました。どちらも呼び出し側では作り直せません。MIME type はバイト列からの
// 推測に頼ることになり（WAV と MP3 は取り違えます）、テキストに至っては復元する手立てが
// ありません。
type Track struct {
	// Audio は生成された音声バイト列です。
	Audio []byte
	// MIMEType は Audio の MIME type です（"audio/mpeg" など）。
	// 保存時の拡張子や Content-Type を決めるのに使います。
	MIMEType string
	// SungLyrics は、音声と一緒に返されたテキストです。
	//
	// lyria-3.5 はここへ自身が組んだ譜面を返します。実測では、区間を [[B1]] [[C2]] のような
	// letter+index のマーカーで表し、歌唱行を [:] で始める独自の記譜でした。歌詞そのものは
	// 渡した表記のまま返ります。
	//
	// 譜面であって、鳴った音の書き起こしではありません。全行が揃った譜面が返る一方で、同じ音を
	// 聴くと一部の行が潰れている、ということが起こります。依頼した歌詞と突き合わせて分かるのは
	// 「モデルが最初から歌う気の無かった行」までで、そこから先は音を聴く側の仕事です。
	//
	// 空文字は異常ではありません。テキストを返さないモデルや、器楽だけの生成があります。
	SungLyrics string
}

// Clone は、呼び出し元が安全に変更できる複製を返します。
//
// 生成結果は singleflight で同時呼び出しの間に共有されます（callguard.Do）。複製せずに
// 返すと、一方の呼び出し元による音声バイト列の書き換えがもう一方へ波及します。
func (t *Track) Clone() *Track {
	if t == nil {
		return nil
	}
	clone := *t
	clone.Audio = cloneBytes(t.Audio)
	return &clone
}
