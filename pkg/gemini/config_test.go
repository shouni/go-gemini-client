package gemini

import (
	"errors"
	"testing"

	"google.golang.org/genai"
)

// ptr はテスト用に float32 のポインタを作成するヘルパーです。
func ptr(f float32) *float32 {
	return &f
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name: "正常系: Gemini API モード",
			config: Config{
				APIKey: "test-api-key",
			},
			wantErr: nil,
		},
		{
			name: "正常系: Vertex AI モード",
			config: Config{
				ProjectID:  "my-project",
				LocationID: "us-central1",
			},
			wantErr: nil,
		},
		{
			name: "正常系: Temperature が境界値(0.0)",
			config: Config{
				APIKey:      "test-api-key",
				Temperature: ptr(0.0),
			},
			wantErr: nil,
		},
		{
			name: "異常系: APIKey と ProjectID が両方存在（排他エラー）",
			config: Config{
				APIKey:    "test-api-key",
				ProjectID: "my-project",
			},
			wantErr: ErrExclusiveConfig,
		},
		{
			name: "異常系: Vertex AI 設定が不完全（ProjectIDのみ）",
			config: Config{
				ProjectID: "my-project",
			},
			wantErr: ErrIncompleteVertexConfig,
		},
		{
			name:    "異常系: 設定が空（必須エラー）",
			config:  Config{},
			wantErr: ErrConfigRequired,
		},
		{
			name: "異常系: Temperature が範囲外(2.1)",
			config: Config{
				APIKey:      "test-api-key",
				Temperature: ptr(2.1),
			},
			wantErr: ErrInvalidTemperature,
		},
		{
			name: "異常系: Temperature が負の値(-0.1)",
			config: Config{
				APIKey:      "test-api-key",
				Temperature: ptr(-0.1),
			},
			wantErr: ErrInvalidTemperature,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("validate() unexpected error = %v", err)
			}
		})
	}
}

func TestConfig_ToClientConfig(t *testing.T) {
	t.Run("Vertex AI への変換", func(t *testing.T) {
		cfg := Config{
			ProjectID:  "proj-v",
			LocationID: "loc-v",
		}
		got := cfg.toClientConfig()
		if got.Project != "proj-v" || got.Location != "loc-v" || got.Backend != genai.BackendVertexAI {
			t.Errorf("toClientConfig() produced invalid Vertex config: %+v", got)
		}
	})

	t.Run("Gemini API への変換", func(t *testing.T) {
		cfg := Config{
			APIKey: "key-g",
		}
		got := cfg.toClientConfig()
		if got.APIKey != "key-g" || got.Backend != genai.BackendGeminiAPI {
			t.Errorf("toClientConfig() produced invalid Gemini config: %+v", got)
		}
	})
}

func TestConfig_GetTemperature(t *testing.T) {
	t.Run("nil の場合はデフォルト値を返す", func(t *testing.T) {
		cfg := Config{Temperature: nil}
		if got := cfg.getTemperature(); got != DefaultTemperature {
			t.Errorf("getTemperature() = %v, want %v", got, DefaultTemperature)
		}
	})

	t.Run("値がある場合はその値を返す", func(t *testing.T) {
		cfg := Config{Temperature: ptr(1.5)}
		if got := cfg.getTemperature(); got != 1.5 {
			t.Errorf("getTemperature() = %v, want 1.5", got)
		}
	})
}
