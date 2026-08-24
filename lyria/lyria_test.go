package lyria

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/shouni/go-gemini-client/gemini"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/time/rate"
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

func (m *MockGeminiClient) GenerateWithAttachments(ctx context.Context, modelName string, prompt string, attachments []gemini.Attachment, opts gemini.GenerateOptions) (*gemini.Response, error) {
	args := m.Called(ctx, modelName, prompt, attachments, opts)
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

func jsonGenerateOptionsWithSeed(t *testing.T, wantSeed *int64) interface{} {
	t.Helper()
	return mock.MatchedBy(func(opts gemini.GenerateOptions) bool {
		if opts.ResponseMIMEType != "application/json" {
			return false
		}
		// 構造化出力スキーマが常に指定されていること
		if opts.ResponseSchema == nil {
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

// TestComposeAttachesModelsAndSeed は、生成されたレシピに呼び出し時の AIModels と
// Seed が付与されることを検証します。レシピの JSON Schema はこれらを意図的に含めて
// おらず（AI に生成させない）、生成後にコードが付けています。ここが抜けると、Seed を
// 指定した再現生成がエラーも警告も無しに効かなくなります。
func TestComposeAttachesModelsAndSeed(t *testing.T) {
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

	ai := AIModels{
		TextModel:   "custom-text-model",
		AudioModel:  "lyria-custom-v1",
		ComposeMode: "jazz",
		Seed:        new(int64),
	}
	*ai.Seed = 42
	lyrics := &LyricsDraft{Title: "Rainy Amsterdam", Lyrics: "Canals reflect the neon lights..."}

	mPrompt.On("GenerateRecipe", "jazz", lyrics).Return("prompt-recipe-text", nil)
	mAI.On("GenerateWithAttachments", mock.Anything, "custom-text-model", "prompt-recipe-text", mock.Anything, jsonGenerateOptionsWithSeed(t, ai.Seed)).Return(&gemini.Response{
		Text: `{"title": "Rainy Amsterdam", "tempo": 85, "mood": "melancholic"}`,
	}, nil)

	recipe, err := workflow.Compose(ctx, ai, lyrics)

	assert.NoError(t, err)
	assert.Equal(t, "Rainy Amsterdam", recipe.Title)
	assert.Equal(t, "custom-text-model", recipe.TextModel)
	assert.Equal(t, "lyria-custom-v1", recipe.AudioModel)
	if assert.NotNil(t, recipe.Seed) {
		assert.Equal(t, int64(42), *recipe.Seed)
	}
	// 共有結果ではなく複製に付与していること（Seed のポインタが使い回されていない）。
	assert.NotSame(t, ai.Seed, recipe.Seed)
	// 歌詞も生成後に付けられる。
	if assert.NotNil(t, recipe.Lyrics) {
		assert.Equal(t, "Rainy Amsterdam", recipe.Lyrics.Title)
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
	mAI.On("GenerateWithAttachments", mock.Anything, "gemini-flash", expectedPrompt, mock.Anything, jsonGenerateOptionsWithSeed(t, nil)).Return(&gemini.Response{
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

	mAI.On("GenerateWithAttachments",
		mock.Anything,
		"lyria-3",
		"custom full prompt",
		mock.Anything,
		mock.Anything).Return(&gemini.Response{Audios: [][]byte{{1, 2, 3}}}, nil)

	audio, err := workflow.GenerateAudio(ctx, &MusicRecipe{Title: "Song"}, nil)

	assert.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, audio)
	mAI.AssertExpectations(t)
}

func TestNewUsesTextRateInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		mAI := new(MockGeminiClient)
		mPrompt := new(MockPromptGen)

		workflow, err := New(mAI, mPrompt, fixedAudioPromptBuilder{fullSong: "full prompt"},
			WithGeminiModel("gemini-flash"),
			WithLyriaModel("lyria-3"),
			WithRateInterval(0),
			WithTextRateInterval(50*time.Millisecond),
		)
		assert.NoError(t, err)

		mPrompt.On("GenerateLyrics", mock.Anything, mock.Anything).Return("prompt-text", nil)
		mAI.On("GenerateWithAttachments", mock.Anything, "gemini-flash", mock.Anything, mock.Anything, mock.Anything).Return(&gemini.Response{
			Text: `{"title":"t","theme":"th","hook":"h","lyrics":"l"}`,
		}, nil).Twice()

		start := time.Now()
		_, err = workflow.GenerateLyrics(ctx, AIModels{}, &CollectedContent{Prompt: "first"})
		assert.NoError(t, err)
		_, err = workflow.GenerateLyrics(ctx, AIModels{}, &CollectedContent{Prompt: "second"})
		assert.NoError(t, err)
		elapsed := time.Since(start)

		// 仮想時計なので、待たされた時間は設定値ちょうどに一致する。
		assert.Equal(t, 50*time.Millisecond, elapsed, "2回目の呼び出しはレート制限で待機するはずです")
	})
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

	mAI.On("GenerateWithAttachments",
		mock.Anything,
		"lyria-3",
		"converted prompt",
		mock.Anything,
		mock.Anything).Return(&gemini.Response{Audios: [][]byte{{1, 2, 3}}}, nil)

	audio, err := workflow.GenerateAudio(ctx, &MusicRecipe{Title: "Song"}, nil)

	assert.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, audio)
	mAI.AssertExpectations(t)
}

func TestGenerateAudioSkipsReadingConverterForEnglish(t *testing.T) {
	ctx := context.Background()
	mAI := new(MockGeminiClient)
	mPrompt := new(MockPromptGen)

	workflow, err := New(mAI, mPrompt, fixedAudioPromptBuilder{fullSong: "english full prompt"},
		WithGeminiModel("gemini-flash"),
		WithLyriaModel("lyria-3"),
		WithRateInterval(0),
		WithReadingConverter(fixedReadingConverter{output: "converted prompt"}),
	)
	assert.NoError(t, err)

	// Lang が "en" の場合、ReadingConverter を通さず元のプロンプトが使われること
	mAI.On("GenerateWithAttachments",
		mock.Anything,
		"lyria-3",
		"english full prompt",
		mock.Anything,
		mock.Anything).Return(&gemini.Response{Audios: [][]byte{{1, 2, 3}}}, nil)

	audio, err := workflow.GenerateAudio(ctx, &MusicRecipe{Title: "Song", AIModels: AIModels{Lang: LangEnglish}}, nil)

	assert.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, audio)
	mAI.AssertExpectations(t)
}

// 構造体リテラルで limiter を省略（nil）した lyriaAudioGenerator でも
// GenerateAudio がパニックせず動作することを確認します（text.go と同じ nil ガード）。
func TestGenerateAudioNilLimiterDoesNotPanic(t *testing.T) {
	ctx := context.Background()
	mAI := new(MockGeminiClient)
	generator := &lyriaAudioGenerator{
		aiClient:          mAI,
		promptBuilder:     fixedAudioPromptBuilder{fullSong: "full prompt"},
		converter:         noopPhoneticConverter{},
		defaultLyriaModel: "lyria-3",
		// limiter は意図的に nil のまま
	}

	mAI.On("GenerateWithAttachments",
		mock.Anything,
		"lyria-3",
		"full prompt",
		mock.Anything,
		mock.Anything).Return(&gemini.Response{Audios: [][]byte{{1, 2, 3}}}, nil)

	audio, err := generator.GenerateAudio(ctx, &MusicRecipe{Title: "Song"}, nil)

	assert.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, audio)
	mAI.AssertExpectations(t)
}

// GenerateLyrics の各エラー分岐（プロンプト生成失敗、AI エラー、nil/空レスポンス、
// 生成後の歌詞空チェック）を、AI が「ゴミ」を返す想定でまとめて検証します。
func TestGenerateLyrics_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	const model = "gemini-flash"
	input := &CollectedContent{Prompt: "context text"}

	tests := []struct {
		name    string
		setup   func(mAI *MockGeminiClient, mPrompt *MockPromptGen)
		wantErr string
	}{
		{
			name: "prompt generation fails",
			setup: func(_ *MockGeminiClient, mPrompt *MockPromptGen) {
				mPrompt.On("GenerateLyrics", mock.Anything, mock.Anything).Return("", errors.New("prompt boom"))
			},
			wantErr: "failed to build lyrics prompt",
		},
		{
			name: "AI returns an error",
			setup: func(mAI *MockGeminiClient, mPrompt *MockPromptGen) {
				mPrompt.On("GenerateLyrics", mock.Anything, mock.Anything).Return("p", nil)
				mAI.On("GenerateWithAttachments", mock.Anything, model, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, errors.New("ai boom"))
			},
			wantErr: "lyrics generation failed",
		},
		{
			name: "AI returns a nil response",
			setup: func(mAI *MockGeminiClient, mPrompt *MockPromptGen) {
				mPrompt.On("GenerateLyrics", mock.Anything, mock.Anything).Return("p", nil)
				mAI.On("GenerateWithAttachments", mock.Anything, model, mock.Anything, mock.Anything, mock.Anything).
					Return((*gemini.Response)(nil), nil)
			},
			wantErr: "lyrics response is nil",
		},
		{
			name: "AI returns an empty string",
			setup: func(mAI *MockGeminiClient, mPrompt *MockPromptGen) {
				mPrompt.On("GenerateLyrics", mock.Anything, mock.Anything).Return("p", nil)
				mAI.On("GenerateWithAttachments", mock.Anything, model, mock.Anything, mock.Anything, mock.Anything).
					Return(&gemini.Response{Text: "   "}, nil)
			},
			wantErr: "AI returned an empty string",
		},
		{
			name: "schema-valid JSON but empty lyrics",
			setup: func(mAI *MockGeminiClient, mPrompt *MockPromptGen) {
				mPrompt.On("GenerateLyrics", mock.Anything, mock.Anything).Return("p", nil)
				mAI.On("GenerateWithAttachments", mock.Anything, model, mock.Anything, mock.Anything, mock.Anything).
					Return(&gemini.Response{Text: `{"title":"t","theme":"th","lyrics":""}`}, nil)
			},
			wantErr: "lyrics draft is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mAI := new(MockGeminiClient)
			mPrompt := new(MockPromptGen)
			tt.setup(mAI, mPrompt)

			g := &lyriaTextGenerator{
				aiClient:     mAI,
				promptGen:    mPrompt,
				defaultModel: model,
			}

			_, err := g.GenerateLyrics(ctx, AIModels{}, input)
			if assert.Error(t, err) {
				assert.True(t, strings.Contains(err.Error(), tt.wantErr),
					"err = %q, want substring %q", err.Error(), tt.wantErr)
			}
			mAI.AssertExpectations(t)
			mPrompt.AssertExpectations(t)
		})
	}
}

// Compose のエラー分岐（プロンプト生成失敗、AI エラー、nil/空レスポンス）を検証します。
func TestCompose_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	const model = "gemini-flash"
	lyrics := &LyricsDraft{Title: "t", Lyrics: "l"}

	tests := []struct {
		name    string
		setup   func(mAI *MockGeminiClient, mPrompt *MockPromptGen)
		wantErr string
	}{
		{
			name: "prompt generation fails",
			setup: func(_ *MockGeminiClient, mPrompt *MockPromptGen) {
				mPrompt.On("GenerateRecipe", mock.Anything, mock.Anything).Return("", errors.New("prompt boom"))
			},
			wantErr: "failed to build prompt",
		},
		{
			name: "AI returns an error",
			setup: func(mAI *MockGeminiClient, mPrompt *MockPromptGen) {
				mPrompt.On("GenerateRecipe", mock.Anything, mock.Anything).Return("p", nil)
				mAI.On("GenerateWithAttachments", mock.Anything, model, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, errors.New("ai boom"))
			},
			wantErr: "compose generation failed",
		},
		{
			name: "AI returns a nil response",
			setup: func(mAI *MockGeminiClient, mPrompt *MockPromptGen) {
				mPrompt.On("GenerateRecipe", mock.Anything, mock.Anything).Return("p", nil)
				mAI.On("GenerateWithAttachments", mock.Anything, model, mock.Anything, mock.Anything, mock.Anything).
					Return((*gemini.Response)(nil), nil)
			},
			wantErr: "compose response is nil",
		},
		{
			name: "AI returns an empty string",
			setup: func(mAI *MockGeminiClient, mPrompt *MockPromptGen) {
				mPrompt.On("GenerateRecipe", mock.Anything, mock.Anything).Return("p", nil)
				mAI.On("GenerateWithAttachments", mock.Anything, model, mock.Anything, mock.Anything, mock.Anything).
					Return(&gemini.Response{Text: ""}, nil)
			},
			wantErr: "AI returned an empty string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mAI := new(MockGeminiClient)
			mPrompt := new(MockPromptGen)
			tt.setup(mAI, mPrompt)

			g := &lyriaTextGenerator{
				aiClient:     mAI,
				promptGen:    mPrompt,
				defaultModel: model,
			}

			_, err := g.Compose(ctx, AIModels{}, lyrics)
			if assert.Error(t, err) {
				assert.True(t, strings.Contains(err.Error(), tt.wantErr),
					"err = %q, want substring %q", err.Error(), tt.wantErr)
			}
			mAI.AssertExpectations(t)
			mPrompt.AssertExpectations(t)
		})
	}
}

func TestMusicRecipeIsJapanese(t *testing.T) {
	assert.True(t, (&MusicRecipe{}).IsJapanese(), "Lang 未指定は日本語扱い")
	assert.True(t, (&MusicRecipe{AIModels: AIModels{Lang: LangJapanese}}).IsJapanese())
	assert.False(t, (&MusicRecipe{AIModels: AIModels{Lang: LangEnglish}}).IsJapanese())
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

	mAI.On("GenerateWithAttachments",
		mock.Anything,
		"lyria-3",
		"full prompt",
		mock.Anything,
		audioGenerateOptionsWithSeed(t, &seed, "")).Return(&gemini.Response{Audios: [][]byte{{1, 2, 3}}}, nil)

	audio, err := generator.GenerateAudio(ctx, &MusicRecipe{Title: "Song", AIModels: AIModels{Seed: &seed}}, nil)

	assert.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, audio)
	mAI.AssertExpectations(t)
}
