package gemini

import (
	"context"
	"errors"
	"testing"
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
			name: "正常系：最小限の設定",
			cfg: Config{
				APIKey: "dummy-key",
			},
			wantErr: nil,
		},
		{
			name: "正常系：Temperatureの上限境界 (1.0)",
			cfg: Config{
				APIKey:      "dummy-key",
				Temperature: ptrFloat(1.0),
			},
			wantErr: nil,
		},
		{
			name: "異常系：APIキーが空",
			cfg: Config{
				APIKey: "",
			},
			wantErr: ErrAPIKeyRequired,
		},
		{
			name: "異常系：Temperatureが範囲外 (2.1)",
			cfg: Config{
				APIKey:      "dummy-key",
				Temperature: ptrFloat(2.1),
			},
			wantErr: ErrInvalidTemperature,
		},
		{
			name: "正常系：リトライ設定のカスタマイズ",
			cfg: Config{
				APIKey:     "dummy-key",
				MaxRetries: 5,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(ctx, tt.cfg)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("エラーが返されるべきですが、nilが返されましたのだ")
				}
				if !errors.Is(err, tt.wantErr) && err.Error() != tt.wantErr.Error() {
					t.Errorf("期待したエラー: %v, 実際のエラー: %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("予期せぬエラーが発生したのだ: %v", err)
			}

			// クライアント内部に正しく値がセットされているか確認
			if tt.cfg.Temperature != nil && client.temperature != *tt.cfg.Temperature {
				t.Errorf("Temperatureが一致しません: got %v, want %v", client.temperature, *tt.cfg.Temperature)
			}
		})
	}
}

func TestGenerateContent_Validation(t *testing.T) {
	// クライアントの初期化（モックなしでバリデーションのみテスト）
	c := &Client{}
	ctx := context.Background()

	t.Run("空のプロンプト", func(t *testing.T) {
		_, err := c.GenerateContent(ctx, "gemini-1.5-flash", "")
		if !errors.Is(err, ErrEmptyPrompt) {
			t.Errorf("ErrEmptyPrompt を期待しましたが %v が返りました", err)
		}
	})
}
