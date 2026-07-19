package lyria

// AIModels selects the text and audio models used by Lyria generation.
type AIModels struct {
	TextModel   string
	AudioModel  string
	LyricsMode  string
	ComposeMode string
	Seed        *int64
	// Lang は歌詞・ボーカルの言語コードです（"ja" / "en"）。空は "ja" 扱いです。
	Lang string `json:"lang,omitempty"`
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

// LangJapanese と LangEnglish は MusicRecipe.Lang に指定できる言語コードです。
const (
	LangJapanese = "ja"
	LangEnglish  = "en"
)

// MusicRecipe describes the song structure and generation settings.
type MusicRecipe struct {
	Title        string         `json:"title"`
	Theme        string         `json:"theme"`
	Mood         string         `json:"mood"`
	Tempo        int            `json:"tempo"`
	Key          string         `json:"key,omitempty"`
	VocalProfile string         `json:"vocal_profile,omitempty"`
	Instruments  []string       `json:"instruments"`
	Sections     []MusicSection `json:"sections"`
	Lyrics       *LyricsDraft   `json:"lyrics,omitempty"`
	AIModels
}

// IsJapanese は、このレシピが日本語楽曲かどうかを返します。Lang 未指定は日本語扱いです。
func (r *MusicRecipe) IsJapanese() bool {
	return r.Lang == "" || r.Lang == LangJapanese
}

// MusicSection describes one section of a song.
type MusicSection struct {
	Name         string `json:"name"`
	Duration     int    `json:"duration_seconds"`
	StartSeconds int    `json:"start_seconds"`
	EndSeconds   int    `json:"end_seconds"`
	Prompt       string `json:"prompt"`
}

// ImagePayload is an optional multimodal image input for audio generation.
type ImagePayload struct {
	Data     []byte
	MIMEType string
}

// CollectedContent is the text and image context used for lyrics and music generation.
type CollectedContent struct {
	Prompt string
	Images []ImagePayload
}
