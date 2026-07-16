package gemini

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/genai"
)

// skipWithoutGCPCredentials は、GCP Application Default Credentials が
// 利用できない環境（CIランナーなど）でこのテストをスキップします。
// Vertex AI バックエンドでの genai.NewClient は ADC を必須とするため、
// 認証情報がない環境ではここでスキップしないと必ず失敗します。
func skipWithoutGCPCredentials(t *testing.T) {
	t.Helper()
	if _, err := google.FindDefaultCredentials(context.Background()); err != nil {
		t.Skipf("GCP Application Default Credentials が見つからないため、このテストをスキップします: %v", err)
	}
}

type fakeModelClient struct {
	calls     int
	gotModel  string
	gotConfig *genai.GenerateContentConfig
	resp      *genai.GenerateContentResponse
	err       error
	errs      []error // 呼び出し順に返すエラー。使い切った後は resp / err に従う
}

func (f *fakeModelClient) GenerateContent(_ context.Context, model string, _ []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	f.calls++
	f.gotModel = model
	f.gotConfig = config
	if f.calls <= len(f.errs) {
		if e := f.errs[f.calls-1]; e != nil {
			return nil, e
		}
	}
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
			if tt.wantErr == nil && tt.cfg.ProjectID != "" {
				// Vertex AI バックエンドの構築には ADC が必要
				skipWithoutGCPCredentials(t)
			}

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

func TestGenerateWithParts_RetriesOnRateLimit(t *testing.T) {
	ctx := context.Background()
	fake := &fakeModelClient{
		errs: []error{genai.APIError{Code: http.StatusTooManyRequests, Status: "RESOURCE_EXHAUSTED"}},
	}
	c := &Client{
		modelClient: fake,
		retryConfig: Config{
			MaxRetries:   2,
			InitialDelay: time.Nanosecond,
			MaxDelay:     time.Nanosecond,
		}.buildRetryConfig(),
	}

	resp, err := c.GenerateWithParts(ctx, "gemini-test", []*genai.Part{{Text: "hello"}}, GenerateOptions{})
	if err != nil {
		t.Fatalf("429 の後にリトライで成功するはずですが、エラーが返りました: %v", err)
	}
	if resp.Text != "ok" {
		t.Fatalf("Text = %q, want ok", resp.Text)
	}
	if fake.calls != 2 {
		t.Fatalf("API 呼び出し回数 = %d, want 2 (初回 + リトライ1回)", fake.calls)
	}
}

func TestGenerateWithParts_DoesNotRetryOnBadRequest(t *testing.T) {
	ctx := context.Background()
	fake := &fakeModelClient{
		err: genai.APIError{Code: http.StatusBadRequest, Status: "INVALID_ARGUMENT"},
	}
	c := &Client{
		modelClient: fake,
		retryConfig: Config{
			MaxRetries:   2,
			InitialDelay: time.Nanosecond,
			MaxDelay:     time.Nanosecond,
		}.buildRetryConfig(),
	}

	_, err := c.GenerateWithParts(ctx, "gemini-test", []*genai.Part{{Text: "hello"}}, GenerateOptions{})
	if err == nil {
		t.Fatal("400 エラーが返るべきですが、nil が返りました")
	}
	if fake.calls != 1 {
		t.Fatalf("API 呼び出し回数 = %d, want 1 (リトライなし)", fake.calls)
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
