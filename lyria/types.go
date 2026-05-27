package lyria

// AIModels selects the text and audio models used by Lyria generation.
type AIModels struct {
	TextModel   string
	AudioModel  string
	LyricsMode  string
	ComposeMode string
	Seed        *int64
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

// MusicRecipe describes the song structure and generation settings.
type MusicRecipe struct {
	Title       string         `json:"title"`
	Theme       string         `json:"theme"`
	Mood        string         `json:"mood"`
	Tempo       int            `json:"tempo"`
	Instruments []string       `json:"instruments"`
	Sections    []MusicSection `json:"sections"`
	Lyrics      *LyricsDraft   `json:"lyrics,omitempty"`
	AIModels
}

// MusicSection describes one section of a song.
type MusicSection struct {
	Name     string `json:"name"`
	Duration int    `json:"duration_seconds"`
	Prompt   string `json:"prompt"`
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

// TextPromptGenerator builds prompts for lyric and recipe generation.
type TextPromptGenerator interface {
	GenerateLyrics(mode string, input string) (string, error)
	GenerateRecipe(mode string, lyrics *LyricsDraft) (string, error)
}

// PromptGenerator is kept as a compatibility alias for TextPromptGenerator.
type PromptGenerator = TextPromptGenerator

// AudioPromptBuilder builds prompts for Lyria audio generation.
type AudioPromptBuilder interface {
	BuildFullSong(recipe *MusicRecipe) string
	BuildSection(recipe *MusicRecipe, section MusicSection) string
}
