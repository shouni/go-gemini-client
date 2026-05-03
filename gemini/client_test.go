package gemini

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/genai"
)

type fakeModelClient struct {
	calls     int
	gotModel  string
	gotConfig *genai.GenerateContentConfig
	resp      *genai.GenerateContentResponse
	err       error
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

func TestNewClient(t *testing.T) {
	ctx := context.Background()

	// ヘルパー関数: float32のポインタを作成
	ptrFloat := func(f float32) *float32 { return &f }

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
			name: "正常系：Temperatureの設定確認",
			cfg: Config{
				APIKey:      "dummy-key",
				Temperature: ptrFloat(0.5),
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
		{
			name: "異常系：Temperatureが範囲外 (2.1)",
			cfg: Config{
				APIKey:      "dummy-key",
				Temperature: ptrFloat(2.1),
			},
			wantErr: ErrInvalidTemperature,
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

			// Temperatureの反映確認 (デフォルト値の考慮)
			expectedTemp := DefaultTemperature
			if tt.cfg.Temperature != nil {
				expectedTemp = *tt.cfg.Temperature
			}
			if client.temperature != expectedTemp {
				t.Errorf("Temperatureが一致しません: got %v, want %v", client.temperature, expectedTemp)
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

func TestBuildGenerateConfig_AppliesOptions(t *testing.T) {
	temp := float32(0.4)
	topP := float32(0.8)
	candidateCount := int32(3)
	seed := int64(12345)
	c := &Client{
		backend:     genai.BackendVertexAI,
		temperature: DefaultTemperature,
	}

	got, err := c.buildGenerateConfig(GenerateOptions{
		SystemPrompt:     "system",
		Temperature:      &temp,
		TopP:             &topP,
		CandidateCount:   &candidateCount,
		ResponseMIMEType: "application/json",
		AspectRatio:      "16:9",
		ImageSize:        "1K",
		Seed:             &seed,
		PersonGeneration: PersonGenerationAllowAll,
	})
	if err != nil {
		t.Fatalf("buildGenerateConfig() unexpected error = %v", err)
	}

	if got.Temperature == nil || *got.Temperature != temp {
		t.Fatalf("Temperature = %v, want %v", got.Temperature, temp)
	}
	if got.TopP == nil || *got.TopP != topP {
		t.Fatalf("TopP = %v, want %v", got.TopP, topP)
	}
	if got.CandidateCount != candidateCount {
		t.Fatalf("CandidateCount = %v, want %v", got.CandidateCount, candidateCount)
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
	c := &Client{temperature: DefaultTemperature}

	got, err := c.buildGenerateConfig(GenerateOptions{
		ResponseMIMEType: "audio/wav",
	})
	if err != nil {
		t.Fatalf("buildGenerateConfig() unexpected error = %v", err)
	}
	if got.ResponseMIMEType != "audio/wav" {
		t.Fatalf("ResponseMIMEType = %q, want audio/wav", got.ResponseMIMEType)
	}
	if len(got.ResponseModalities) != 2 || got.ResponseModalities[0] != "AUDIO" || got.ResponseModalities[1] != "TEXT" {
		t.Fatalf("ResponseModalities = %v, want [AUDIO TEXT]", got.ResponseModalities)
	}
}

func TestBuildGenerateConfig_Validation(t *testing.T) {
	c := &Client{temperature: DefaultTemperature}
	invalidTemp := float32(2.1)
	invalidTopP := float32(1.1)
	invalidCandidateCount := int32(0)
	invalidSeed := int64(1) << 40

	tests := []struct {
		name    string
		opts    GenerateOptions
		wantErr error
	}{
		{"Temperature範囲外", GenerateOptions{Temperature: &invalidTemp}, ErrInvalidTemperature},
		{"TopP範囲外", GenerateOptions{TopP: &invalidTopP}, ErrInvalidTopP},
		{"CandidateCount範囲外", GenerateOptions{CandidateCount: &invalidCandidateCount}, ErrInvalidCandidateCount},
		{"Seed範囲外", GenerateOptions{Seed: &invalidSeed}, ErrInvalidSeed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.buildGenerateConfig(tt.opts)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("buildGenerateConfig() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGenerateWithParts_UsesInternalModelClient(t *testing.T) {
	ctx := context.Background()
	topP := float32(0.7)
	fake := &fakeModelClient{}
	c := &Client{
		modelClient: fake,
		retryConfig: Config{
			MaxRetries:   1,
			InitialDelay: time.Nanosecond,
			MaxDelay:     time.Nanosecond,
		}.buildRetryConfig(),
		temperature: DefaultTemperature,
	}

	resp, err := c.GenerateWithParts(ctx, "gemini-test", []*genai.Part{{Text: "hello"}}, GenerateOptions{TopP: &topP})
	if err != nil {
		t.Fatalf("GenerateWithParts() unexpected error = %v", err)
	}
	if resp.Text != "ok" {
		t.Fatalf("Response.Text = %q, want ok", resp.Text)
	}
	if fake.calls != 1 {
		t.Fatalf("GenerateContent calls = %d, want 1", fake.calls)
	}
	if fake.gotModel != "gemini-test" {
		t.Fatalf("model = %q, want gemini-test", fake.gotModel)
	}
	if fake.gotConfig == nil || fake.gotConfig.TopP == nil || *fake.gotConfig.TopP != topP {
		t.Fatalf("TopP was not passed to model client: %+v", fake.gotConfig)
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
		temperature: DefaultTemperature,
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
