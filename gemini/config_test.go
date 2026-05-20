package gemini

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/genai"
)

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

func TestConfig_buildRetryConfig(t *testing.T) {
	t.Run("デフォルト値が適用されること", func(t *testing.T) {
		cfg := Config{}
		got := cfg.buildRetryConfig()
		if got.MaxRetries != DefaultMaxRetries {
			t.Errorf("MaxRetries = %v, want %v", got.MaxRetries, DefaultMaxRetries)
		}
	})

	t.Run("設定値で上書きされること", func(t *testing.T) {
		cfg := Config{
			MaxRetries:   5,
			InitialDelay: 10 * time.Second,
			MaxDelay:     60 * time.Second,
		}
		got := cfg.buildRetryConfig()
		if got.MaxRetries != 5 || got.InitialInterval != 10*time.Second || got.MaxInterval != 60*time.Second {
			t.Errorf("設定が正しく適用されていません: %+v", got)
		}
	})
}

func TestConfig_FilePolling(t *testing.T) {
	t.Run("デフォルト値が適用されること", func(t *testing.T) {
		cfg := Config{}
		if got := cfg.getFilePollingInterval(); got != PollingInterval {
			t.Errorf("getFilePollingInterval() = %v, want %v", got, PollingInterval)
		}
		if got := cfg.getFilePollingTimeout(); got != PollingTimeout {
			t.Errorf("getFilePollingTimeout() = %v, want %v", got, PollingTimeout)
		}
	})

	t.Run("設定値で上書きされること", func(t *testing.T) {
		cfg := Config{
			FilePollingInterval: 500 * time.Millisecond,
			FilePollingTimeout:  5 * time.Second,
		}
		if got := cfg.getFilePollingInterval(); got != 500*time.Millisecond {
			t.Errorf("getFilePollingInterval() = %v, want %v", got, 500*time.Millisecond)
		}
		if got := cfg.getFilePollingTimeout(); got != 5*time.Second {
			t.Errorf("getFilePollingTimeout() = %v, want %v", got, 5*time.Second)
		}
	})
}
