package gemini

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/genai"
)

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
			wantErr: ErrIncompleteVertexConfig, // 指摘通り、詳細なエラーを期待するように修正
		},
		{
			name: "異常系：ProjectID と APIKey の両方が設定されている",
			cfg: Config{
				APIKey:     "dummy-key",
				ProjectID:  "my-project",
				LocationID: "asia-northeast1",
			},
			wantErr: ErrExclusiveConfig, // 排他制御のテスト
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
			} else if tt.cfg.APIKey != "" {
				if client.backend != genai.BackendGeminiAPI {
					t.Errorf("BackendがGemini APIになっていません: got %v", client.backend)
				}
			}

			// クライアント内部に正しく値がセットされているか確認
			if tt.cfg.Temperature != nil && client.temperature != *tt.cfg.Temperature {
				t.Errorf("Temperatureが一致しません: got %v, want %v", client.temperature, *tt.cfg.Temperature)
			}
		})
	}
}

func TestGenerateContent_Validation(t *testing.T) {
	ctx := context.Background()
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
