// Package music は、楽曲構成を表すデータ型（Recipe とその周辺）を提供します。
//
// これらの型は lyria の生成ワークフローだけでなく、動画生成・履歴表示など
// 多くの下流サービスが「楽曲の語彙」として共有します。ワークフロー本体
// （レート制限や singleflight を伴う lyria パッケージ）を輸入せずに型だけを
// 参照できるよう、依存を持たない葉パッケージとして分離しています。
// lyria パッケージは互換のため MusicRecipe = music.Recipe などの別名を公開しています。
package music

import "slices"

// AIModels selects the text and audio models used by music generation.
//
// Recipe に埋め込まれるため、JSON タグは Recipe の他のフィールドと
// 同じ snake_case に揃えています。タグを省くと Go のフィールド名がそのまま
// レシピ JSON に出力されてしまいます。
type AIModels struct {
	TextModel   string `json:"text_model,omitempty"`
	AudioModel  string `json:"audio_model,omitempty"`
	LyricsMode  string `json:"lyrics_mode,omitempty"`
	ComposeMode string `json:"compose_mode,omitempty"`
	Seed        *int64 `json:"seed,omitempty"`
	// Lang は歌詞・ボーカルの言語コードです（"ja" / "en"）。空は "ja" 扱いです。
	Lang string `json:"lang,omitempty"`
	// LyricReading は、日本語詞を音声生成へ渡すときの表記です（LyricReadingKana /
	// LyricReadingOriginal）。空は LyricReadingKana 扱いです。
	//
	// レシピに載せるのは、プロンプトを組む側に届く情報がレシピだけだからです。ここに
	// 無ければ曲ごとに切り替えられず、recipe.json に残らないので作り直しでも再現できません。
	// 英語詞には効きません（読み変換を通らないため）。
	LyricReading string `json:"lyric_reading,omitempty"`
}

// LyricReadingKana と LyricReadingOriginal は AIModels.LyricReading に指定できる値です。
//
// Kana は日本語詞を読み表記（カタカナ）へ変換してから渡します。漢字の読み違いを防ぐ既定です。
// Original は書かれたままの表記で渡します。モデルが漢字を読めるなら、読み表記へ潰したときに
// 失われる語の区切りと意味が発音の手がかりとして残ります。どちらが良いかはモデルで変わる
// ので、同じレシピで両方を試せるよう値として持ちます。
const (
	LyricReadingKana     = "kana"
	LyricReadingOriginal = "original"
)

// SendsLyricsAsWritten は、日本語詞を変換せず書かれたままの表記で渡すかを返します。
func (m AIModels) SendsLyricsAsWritten() bool {
	return m.LyricReading == LyricReadingOriginal
}

// LyricsDraft is the structured lyric output used by recipe composition.
type LyricsDraft struct {
	Title     string   `json:"title"`
	Theme     string   `json:"theme"`
	Hook      string   `json:"hook"`
	Lyrics    string   `json:"lyrics"`
	Keywords  []string `json:"keywords,omitempty"`
	Mood      string   `json:"mood,omitempty"`
	Narrative string   `json:"narrative,omitempty"`
}

// Clone は LyricsDraft を呼び出し元が安全に変更できるように複製します。
func (d *LyricsDraft) Clone() *LyricsDraft {
	if d == nil {
		return nil
	}

	dst := *d
	dst.Keywords = slices.Clone(d.Keywords)
	return &dst
}

// LangJapanese と LangEnglish は Recipe.Lang に指定できる言語コードです。
const (
	LangJapanese = "ja"
	LangEnglish  = "en"
)

// Recipe describes the song structure and generation settings.
type Recipe struct {
	Title        string       `json:"title"`
	Theme        string       `json:"theme"`
	Mood         string       `json:"mood"`
	Tempo        int          `json:"tempo"`
	Key          string       `json:"key,omitempty"`
	VocalProfile string       `json:"vocal_profile,omitempty"`
	Instruments  []string     `json:"instruments"`
	Sections     []Section    `json:"sections"`
	Lyrics       *LyricsDraft `json:"lyrics,omitempty"`
	AIModels
}

// IsJapanese は、このレシピが日本語楽曲かどうかを返します。Lang 未指定は日本語扱いです。
func (r *Recipe) IsJapanese() bool {
	return r.Lang == "" || r.Lang == LangJapanese
}

// Clone は Recipe と内部のスライスやポインタを複製します。
//
// スライスは slices.Clone で写します。nil は nil、空は空のまま残るため、複製を
// 経由しても JSON のワイヤ形式（instruments の null と []）が変わりません。
func (r *Recipe) Clone() *Recipe {
	if r == nil {
		return nil
	}

	dst := *r
	dst.Instruments = slices.Clone(r.Instruments)
	dst.Sections = slices.Clone(r.Sections)
	dst.Lyrics = r.Lyrics.Clone()
	if r.Seed != nil {
		v := *r.Seed
		dst.Seed = &v
	}
	return &dst
}

// Section describes one section of a song.
type Section struct {
	Name         string `json:"name"`
	Duration     int    `json:"duration_seconds"`
	StartSeconds int    `json:"start_seconds"`
	EndSeconds   int    `json:"end_seconds"`
	Prompt       string `json:"prompt"`
}
