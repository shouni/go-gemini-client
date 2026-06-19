package gemini

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/genai"
)

type fakeModelClient struct {
	calls              int
	editCalls          int
	gotModel           string
	gotPrompt          string
	gotConfig          *genai.GenerateContentConfig
	gotEditConfig      *genai.EditImageConfig
	gotReferenceImages []genai.ReferenceImage
	resp               *genai.GenerateContentResponse
	editResp           *genai.EditImageResponse
	err                error
}

func (f *fakeModelClient) GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	f.calls++
	f.gotModel = model
	f.gotConfig = config
	if f.err != nil {
		return nil, f.err
	}
	if f.resp != nil {
		return f.resp, nil
	}
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				FinishReason: genai.FinishReasonStop,
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "ok"}},
				},
			},
		},
	}, nil
}

func (f *fakeModelClient) EditImage(ctx context.Context, model string, prompt string, referenceImages []genai.ReferenceImage, config *genai.EditImageConfig) (*genai.EditImageResponse, error) {
	f.editCalls++
	f.gotModel = model
	f.gotPrompt = prompt
	f.gotReferenceImages = referenceImages
	f.gotEditConfig = config
	if f.err != nil {
		return nil, f.err
	}
	if f.editResp != nil {
		return f.editResp, nil
	}
	return &genai.EditImageResponse{}, nil
}

func TestNewClient(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{
			name: "正常系：Gemini API モード (API Key)",
			cfg: Config{
				APIKey: "dummy-key",
			},
			wantErr: nil,
		},
		{
			name: "正常系：Vertex AI モード (Project & Location)",
			cfg: Config{
				ProjectID:  "my-project",
				LocationID: "us-central1",
			},
			wantErr: nil,
		},
		{
			name: "異常系：設定が完全に空",
			cfg: Config{
				APIKey:     "",
				ProjectID:  "",
				LocationID: "",
			},
			wantErr: ErrConfigRequired,
		},
		{
			name: "異常系：Vertex AI 設定が不完全 (Location欠損)",
			cfg: Config{
				ProjectID: "my-project",
			},
			wantErr: ErrIncompleteVertexConfig,
		},
		{
			name: "異常系：ProjectID と APIKey の両方が設定されている",
			cfg: Config{
				APIKey:     "dummy-key",
				ProjectID:  "my-project",
				LocationID: "asia-northeast1",
			},
			wantErr: ErrExclusiveConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(ctx, tt.cfg)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("エラーが返されるべきですが、nilが返されました")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("期待したエラー: %v, 実際のエラー: %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("予期せぬエラーが発生しました: %v", err)
			}

			// Backend 型のチェック
			if tt.cfg.ProjectID != "" {
				if client.backend != genai.BackendVertexAI {
					t.Errorf("BackendがVertex AIになっていません: got %v", client.backend)
				}
				if !client.IsVertexAI() {
					t.Error("IsVertexAI() が false を返しました")
				}
			} else if tt.cfg.APIKey != "" {
				if client.backend != genai.BackendGeminiAPI {
					t.Errorf("BackendがGemini APIになっていません: got %v", client.backend)
				}
				if client.IsVertexAI() {
					t.Error("IsVertexAI() が true を返しました")
				}
			}
		})
	}
}

func TestGenerateContent_Validation(t *testing.T) {
	ctx := context.Background()
	// クライアント作成自体が失敗しない最小構成
	cfg := Config{APIKey: "dummy-key"}
	c, err := NewClient(ctx, cfg)
	if err != nil {
		t.Fatalf("クライアントの初期化に失敗しました: %v", err)
	}

	t.Run("空のプロンプト", func(t *testing.T) {
		_, err := c.GenerateContent(ctx, "gemini-1.5-flash", "")
		if !errors.Is(err, ErrEmptyPrompt) {
			t.Errorf("ErrEmptyPrompt を期待しましたが %v が返りました", err)
		}
	})
}

func TestGenerateWithParts_Validation(t *testing.T) {
	ctx := context.Background()
	c := &Client{}

	tests := []struct {
		name      string
		modelName string
		parts     []*genai.Part
		wantErr   error
	}{
		{
			name:      "モデル名が空",
			modelName: "",
			parts:     []*genai.Part{{Text: "hello"}},
			wantErr:   ErrEmptyModelName,
		},
		{
			name:      "パーツが空",
			modelName: "gemini-test",
			parts:     nil,
			wantErr:   ErrEmptyParts,
		},
		{
			name:      "nilパーツを含む",
			modelName: "gemini-test",
			parts:     []*genai.Part{nil},
			wantErr:   ErrInvalidPart,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.GenerateWithParts(ctx, tt.modelName, tt.parts, GenerateOptions{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("GenerateWithParts() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestEditImage_Validation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		client    *Client
		modelName string
		prompt    string
		wantErr   error
	}{
		{
			name:      "モデル名が空",
			client:    &Client{backend: genai.BackendVertexAI},
			modelName: "",
			prompt:    "edit",
			wantErr:   ErrEmptyModelName,
		},
		{
			name:      "プロンプトが空",
			client:    &Client{backend: genai.BackendVertexAI},
			modelName: "imagen-edit",
			prompt:    "",
			wantErr:   ErrEmptyPrompt,
		},
		{
			name:      "Gemini API backend は非対応",
			client:    &Client{backend: genai.BackendGeminiAPI},
			modelName: "imagen-edit",
			prompt:    "edit",
			wantErr:   ErrUnsupportedBackend,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.client.EditImage(ctx, tt.modelName, tt.prompt, nil, nil)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("EditImage() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestEditImage_CallsSDKInVertexAI(t *testing.T) {
	ctx := context.Background()
	fake := &fakeModelClient{
		editResp: &genai.EditImageResponse{
			GeneratedImages: []*genai.GeneratedImage{{}},
		},
	}
	cfg := &genai.EditImageConfig{
		NumberOfImages: 1,
		AspectRatio:    "1:1",
	}
	referenceImages := []genai.ReferenceImage{
		genai.NewRawReferenceImage(nil, 1),
		genai.NewMaskReferenceImage(nil, 2, nil),
	}
	c := &Client{
		modelClient: fake,
		backend:     genai.BackendVertexAI,
		retryConfig: Config{
			MaxRetries:   1,
			InitialDelay: time.Nanosecond,
			MaxDelay:     time.Nanosecond,
		}.buildRetryConfig(),
	}

	resp, err := c.EditImage(ctx, "imagen-edit", "replace the background", referenceImages, cfg)
	if err != nil {
		t.Fatalf("EditImage() unexpected error = %v", err)
	}
	if resp != fake.editResp {
		t.Fatal("EditImage() did not return SDK response")
	}
	if fake.editCalls != 1 {
		t.Fatalf("EditImage() calls = %d, want 1", fake.editCalls)
	}
	if fake.gotModel != "imagen-edit" || fake.gotPrompt != "replace the background" {
		t.Fatalf("EditImage() forwarded model/prompt incorrectly: model=%q prompt=%q", fake.gotModel, fake.gotPrompt)
	}
	if fake.gotEditConfig != cfg {
		t.Fatal("EditImage() did not forward config")
	}
	if len(fake.gotReferenceImages) != 2 || fake.gotReferenceImages[0] != referenceImages[0] || fake.gotReferenceImages[1] != referenceImages[1] {
		t.Fatalf("EditImage() did not forward reference images: %+v", fake.gotReferenceImages)
	}
}

func TestBuildGenerateConfig_AppliesOptions(t *testing.T) {
	seed := int64(12345)
	c := &Client{
		backend: genai.BackendVertexAI,
	}

	got, err := c.buildGenerateConfig(GenerateOptions{
		SystemPrompt:     "system",
		ResponseMIMEType: "application/json",
		AspectRatio:      "16:9",
		ImageSize:        "1K",
		Seed:             &seed,
		PersonGeneration: PersonGenerationAllowAll,
	})
	if err != nil {
		t.Fatalf("buildGenerateConfig() unexpected error = %v", err)
	}

	if got.Seed == nil || *got.Seed != int32(seed) {
		t.Fatalf("Seed = %v, want %v", got.Seed, seed)
	}
	if got.SystemInstruction == nil || len(got.SystemInstruction.Parts) != 1 || got.SystemInstruction.Parts[0].Text != "system" {
		t.Fatalf("SystemInstruction was not applied: %+v", got.SystemInstruction)
	}
	if got.ResponseMIMEType != "application/json" {
		t.Fatalf("ResponseMIMEType = %q, want application/json", got.ResponseMIMEType)
	}
	if got.ImageConfig == nil || got.ImageConfig.AspectRatio != "16:9" || got.ImageConfig.ImageSize != "1K" || got.ImageConfig.PersonGeneration != string(PersonGenerationAllowAll) {
		t.Fatalf("ImageConfig was not applied: %+v", got.ImageConfig)
	}
}

func TestBuildGenerateConfig_AudioResponseMIMETypeSetsModalities(t *testing.T) {
	c := &Client{}

	got, err := c.buildGenerateConfig(GenerateOptions{
		ResponseMIMEType: "audio/wav",
	})
	if err != nil {
		t.Fatalf("buildGenerateConfig() unexpected error = %v", err)
	}
	if got.ResponseMIMEType != "audio/wav" {
		t.Fatalf("ResponseMIMEType = %q, want audio/wav", got.ResponseMIMEType)
	}
	if len(got.ResponseModalities) != 1 || got.ResponseModalities[0] != "AUDIO" {
		t.Fatalf("ResponseModalities = %v, want [AUDIO]", got.ResponseModalities)
	}
}

func TestGenerateWithParts_AudioOnlyResponse(t *testing.T) {
	ctx := context.Background()
	fake := &fakeModelClient{
		resp: &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					FinishReason: genai.FinishReasonStop,
					Content: &genai.Content{
						Parts: []*genai.Part{
							{InlineData: &genai.Blob{MIMEType: "audio/wav", Data: []byte("only-audio")}},
						},
					},
				},
			},
		},
	}
	c := &Client{
		modelClient: fake,
		retryConfig: Config{MaxRetries: 1}.buildRetryConfig(),
	}

	resp, err := c.GenerateWithParts(ctx, "gemini-test", []*genai.Part{{Text: "voice please"}}, GenerateOptions{
		ResponseMIMEType: "audio/wav",
	})
	if err != nil {
		t.Fatalf("音声のみのレスポンスでエラーが発生しました: %v", err)
	}
	if resp.Text != "" {
		t.Fatalf("Text は空であるべきです: got %q", resp.Text)
	}
	if len(resp.Audios) != 1 || string(resp.Audios[0]) != "only-audio" {
		t.Fatalf("音声データが正しく抽出されていません: %v", resp.Audios)
	}
}

func TestGenerateWithParts_ExtractsImagesAndAudios(t *testing.T) {
	ctx := context.Background()
	fake := &fakeModelClient{
		resp: &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					FinishReason: genai.FinishReasonStop,
					Content: &genai.Content{
						Parts: []*genai.Part{
							{Text: "ok"},
							{InlineData: &genai.Blob{MIMEType: "image/png", Data: []byte("image")}},
							{InlineData: &genai.Blob{MIMEType: "audio/wav", Data: []byte("audio")}},
						},
					},
				},
			},
		},
	}
	c := &Client{
		modelClient: fake,
		retryConfig: Config{
			MaxRetries:   1,
			InitialDelay: time.Nanosecond,
			MaxDelay:     time.Nanosecond,
		}.buildRetryConfig(),
	}

	resp, err := c.GenerateWithParts(ctx, "gemini-test", []*genai.Part{{Text: "hello"}}, GenerateOptions{})
	if err != nil {
		t.Fatalf("GenerateWithParts() unexpected error = %v", err)
	}
	if len(resp.Images) != 1 || string(resp.Images[0]) != "image" {
		t.Fatalf("Images = %v, want image", resp.Images)
	}
	if len(resp.Audios) != 1 || string(resp.Audios[0]) != "audio" {
		t.Fatalf("Audios = %v, want audio", resp.Audios)
	}
}

func TestGenerateOptions_HasImageConfig(t *testing.T) {
	tests := []struct {
		name string
		opts GenerateOptions
		want bool
	}{
		{"設定なし", GenerateOptions{}, false},
		{"AspectRatioあり", GenerateOptions{AspectRatio: "16:9"}, true},
		{"ImageSizeあり", GenerateOptions{ImageSize: "1K"}, true},
		{"PersonGenerationあり", GenerateOptions{PersonGeneration: PersonGenerationAllowAll}, true},
		{"その他のみ", GenerateOptions{SystemPrompt: "test"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.HasImageConfig(); got != tt.want {
				t.Errorf("HasImageConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}
