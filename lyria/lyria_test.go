package lyria

import (
	"context"
	"testing"

	"github.com/shouni/go-gemini-client/gemini"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/time/rate"
	"google.golang.org/genai"
)

// --- Mocks ---

type MockGeminiClient struct {
	mock.Mock
}

func (m *MockGeminiClient) GenerateContent(ctx context.Context, model, prompt string) (*gemini.Response, error) {
	args := m.Called(ctx, model, prompt)
	if res, ok := args.Get(0).(*gemini.Response); ok {
		return res, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockGeminiClient) GenerateWithParts(ctx context.Context, modelName string, parts []*genai.Part, opts gemini.GenerateOptions) (*gemini.Response, error) {
	args := m.Called(ctx, modelName, parts, opts)
	if res, ok := args.Get(0).(*gemini.Response); ok {
		return res, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockGeminiClient) IsVertexAI() bool {
	args := m.Called()
	return args.Bool(0)
}

type MockPromptGen struct {
	mock.Mock
}

type noopPhoneticConverter struct{}

func (noopPhoneticConverter) ConvertToReading(input string) string {
	return input
}

type fixedReadingConverter struct {
	output string
}

func (c fixedReadingConverter) ConvertToReading(string) string {
	return c.output
}

type fixedAudioPromptBuilder struct {
	fullSong string
}

func (b fixedAudioPromptBuilder) BuildFullSong(*MusicRecipe) string {
	return b.fullSong
}

func partsWithText(t *testing.T, want string) interface{} {
	t.Helper()
	return mock.MatchedBy(func(parts []*genai.Part) bool {
		return len(parts) == 1 && parts[0] != nil && parts[0].Text == want
	})
}

func jsonGenerateOptionsWithSeed(t *testing.T, wantSeed *int64) interface{} {
	t.Helper()
	return mock.MatchedBy(func(opts gemini.GenerateOptions) bool {
		if opts.ResponseMIMEType != "application/json" {
			return false
		}
		if wantSeed == nil {
			return opts.Seed == nil
		}
		return opts.Seed != nil && *opts.Seed == *wantSeed
	})
}

func audioGenerateOptionsWithSeed(t *testing.T, wantSeed *int64, wantMIMEType string) interface{} {
	t.Helper()
	return mock.MatchedBy(func(opts gemini.GenerateOptions) bool {
		if opts.ResponseMIMEType != wantMIMEType {
			return false
		}
		if wantSeed == nil {
			return opts.Seed == nil
		}
		return opts.Seed != nil && *opts.Seed == *wantSeed
	})
}

// GenerateLyrics に mode 引数を追加
func (m *MockPromptGen) GenerateLyrics(mode string, input string) (string, error) {
	args := m.Called(mode, input)
	return args.String(0), args.Error(1)
}

func (m *MockPromptGen) GenerateRecipe(mode string, lyrics *LyricsDraft) (string, error) {
	args := m.Called(mode, lyrics)
	return args.String(0), args.Error(1)
}

func (m *MockPromptGen) GenerateCoverArt(mode string, recipe *MusicRecipe) (string, error) {
	args := m.Called(mode, recipe)
	return args.String(0), args.Error(1)
}

// --- Tests ---

func TestWorkflow_Run(t *testing.T) {
	ctx := context.Background()
	mAI := new(MockGeminiClient)
	mPrompt := new(MockPromptGen)
	textGenerator := &lyriaTextGenerator{
		aiClient:     mAI,
		promptGen:    mPrompt,
		defaultModel: "gemini-flash",
	}

	// テスト対象のワークフローを構築
	workflow := &Workflow{
		lyricist: textGenerator,
		composer: textGenerator,
		audio: &lyriaAudioGenerator{
			aiClient:          mAI,
			defaultLyriaModel: "lyria-3",
			limiter:           rate.NewLimiter(rate.Inf, 0),
			promptBuilder:     fixedAudioPromptBuilder{fullSong: "full prompt"},
			converter:         noopPhoneticConverter{},
		},
	}

	ai := AIModels{
		TextModel:   "custom-text-model",
		AudioModel:  "lyria-custom-v1",
		LyricsMode:  "outro",
		ComposeMode: "jazz",
		Seed:        new(int64),
	}
	*ai.Seed = 42
	contextText := "雨のアムステルダム"
	input := &CollectedContent{
		Prompt: contextText,
	}

	// 期待される中間データ
	expectedLyrics := &LyricsDraft{
		Title:  "Rainy Amsterdam",
		Theme:  "Neon reflection on canals",
		Lyrics: "Canals reflect the neon lights...",
	}

	lyricsJSON := `{"title": "Rainy Amsterdam", "theme": "Neon reflection on canals", "lyrics": "Canals reflect the neon lights..."}`
	recipeJSON := `{"title": "Rainy Amsterdam", "tempo": 85, "mood": "melancholic"}`
	fakeWav := []byte("RIFF....WAVEfmt....data")

	// 1. 作詞プロンプト生成
	mPrompt.On("GenerateLyrics", "outro", contextText).Return("prompt-lyrics-text", nil)
	mAI.On("GenerateWithParts", mock.Anything, "custom-text-model", partsWithText(t, "prompt-lyrics-text"), jsonGenerateOptionsWithSeed(t, ai.Seed)).Return(&gemini.Response{
		Text: "```json\n" + lyricsJSON + "\n```",
	}, nil)

	// 2. 作曲レシピ生成
	mPrompt.On("GenerateRecipe", "jazz", expectedLyrics).Return("prompt-recipe-text", nil)
	mAI.On("GenerateWithParts", mock.Anything, "custom-text-model", partsWithText(t, "prompt-recipe-text"), jsonGenerateOptionsWithSeed(t, ai.Seed)).Return(&gemini.Response{
		Text: recipeJSON,
	}, nil)

	// 3. 音声生成実行
	mAI.On("GenerateWithParts", mock.Anything, "lyria-custom-v1", mock.Anything, mock.Anything).Return(&gemini.Response{
		Audios: [][]byte{fakeWav},
	}, nil)

	// 実行
	recipe, wav, err := workflow.Run(ctx, ai, input)

	// 検証
	assert.NoError(t, err)
	assert.NotNil(t, recipe)
	assert.Equal(t, "Rainy Amsterdam", recipe.Title)
	assert.Equal(t, 85, recipe.Tempo)
	assert.Equal(t, fakeWav, wav)

	if assert.NotNil(t, recipe.Seed) {
		assert.Equal(t, int64(42), *recipe.Seed)
	}

	mPrompt.AssertExpectations(t)
	mAI.AssertExpectations(t)
}

func TestWorkflow_Compose(t *testing.T) {
	ctx := context.Background()
	mAI := new(MockGeminiClient)
	mPrompt := new(MockPromptGen)

	workflow := &Workflow{
		composer: &lyriaTextGenerator{
			aiClient:     mAI,
			promptGen:    mPrompt,
			defaultModel: "gemini-flash",
		},
	}

	lyrics := &LyricsDraft{Title: "Lofi Beats", Lyrics: "Chill vibes only"}
	mode := "lofi"
	expectedPrompt := "Build me a lofi recipe"
	rawJSON := `{
		"title": "Lofi Chill",
		"tempo": 70,
		"mood": "relaxed",
		"key": "D minor",
		"vocal_profile": "Japanese female vocal, clear diction",
		"sections": [
			{
				"name": "Verse",
				"duration_seconds": 30,
				"start_seconds": 0,
				"end_seconds": 30,
				"prompt": "soft opening groove"
			}
		]
	}`

	mPrompt.On("GenerateRecipe", mode, lyrics).Return(expectedPrompt, nil)
	mAI.On("GenerateWithParts", mock.Anything, "gemini-flash", partsWithText(t, expectedPrompt), jsonGenerateOptionsWithSeed(t, nil)).Return(&gemini.Response{
		Text: rawJSON,
	}, nil)

	recipe, err := workflow.Compose(ctx, AIModels{
		TextModel:   "gemini-flash",
		ComposeMode: mode,
	}, lyrics)

	assert.NoError(t, err)
	assert.NotNil(t, recipe)
	assert.Equal(t, 70, recipe.Tempo)
	assert.Equal(t, "Lofi Chill", recipe.Title)
	assert.Equal(t, "D minor", recipe.Key)
	assert.Equal(t, "Japanese female vocal, clear diction", recipe.VocalProfile)
	if assert.Len(t, recipe.Sections, 1) {
		assert.Equal(t, 0, recipe.Sections[0].StartSeconds)
		assert.Equal(t, 30, recipe.Sections[0].EndSeconds)
	}

	mPrompt.AssertExpectations(t)
	mAI.AssertExpectations(t)
}

func TestNewUsesAudioPromptBuilder(t *testing.T) {
	ctx := context.Background()
	mAI := new(MockGeminiClient)
	mPrompt := new(MockPromptGen)

	workflow, err := New(mAI, mPrompt, fixedAudioPromptBuilder{fullSong: "custom full prompt"},
		WithGeminiModel("gemini-flash"),
		WithLyriaModel("lyria-3"),
		WithRateInterval(0),
	)
	assert.NoError(t, err)

	mAI.On("GenerateWithParts",
		mock.Anything,
		"lyria-3",
		partsWithText(t, "custom full prompt"),
		mock.Anything,
	).Return(&gemini.Response{Audios: [][]byte{{1, 2, 3}}}, nil)

	audio, err := workflow.GenerateAudio(ctx, &MusicRecipe{Title: "Song"}, nil)

	assert.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, audio)
	mAI.AssertExpectations(t)
}

func TestNewUsesReadingConverterOption(t *testing.T) {
	ctx := context.Background()
	mAI := new(MockGeminiClient)
	mPrompt := new(MockPromptGen)

	workflow, err := New(mAI, mPrompt, fixedAudioPromptBuilder{fullSong: "漢字 prompt"},
		WithGeminiModel("gemini-flash"),
		WithLyriaModel("lyria-3"),
		WithRateInterval(0),
		WithReadingConverter(fixedReadingConverter{output: "converted prompt"}),
	)
	assert.NoError(t, err)

	mAI.On("GenerateWithParts",
		mock.Anything,
		"lyria-3",
		partsWithText(t, "converted prompt"),
		mock.Anything,
	).Return(&gemini.Response{Audios: [][]byte{{1, 2, 3}}}, nil)

	audio, err := workflow.GenerateAudio(ctx, &MusicRecipe{Title: "Song"}, nil)

	assert.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, audio)
	mAI.AssertExpectations(t)
}

func TestGenerateAudioKeepsSeed(t *testing.T) {
	ctx := context.Background()
	mAI := new(MockGeminiClient)
	seed := int64(42)
	generator := &lyriaAudioGenerator{
		aiClient:          mAI,
		promptBuilder:     fixedAudioPromptBuilder{fullSong: "full prompt"},
		converter:         noopPhoneticConverter{},
		defaultLyriaModel: "lyria-3",
		limiter:           rate.NewLimiter(rate.Inf, 0),
	}

	mAI.On("GenerateWithParts",
		mock.Anything,
		"lyria-3",
		partsWithText(t, "full prompt"),
		audioGenerateOptionsWithSeed(t, &seed, ""),
	).Return(&gemini.Response{Audios: [][]byte{{1, 2, 3}}}, nil)

	audio, err := generator.GenerateAudio(ctx, &MusicRecipe{Title: "Song", AIModels: AIModels{Seed: &seed}}, nil)

	assert.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, audio)
	mAI.AssertExpectations(t)
}
