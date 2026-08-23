package gemini

import (
	"errors"
	"net/http"
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
		got, err := cfg.toClientConfig()
		if err != nil {
			t.Fatalf("toClientConfig() error = %v", err)
		}
		if got.Project != "proj-v" || got.Location != "loc-v" || got.Backend != genai.BackendVertexAI {
			t.Errorf("toClientConfig() produced invalid Vertex config: %+v", got)
		}
	})

	t.Run("Gemini API への変換", func(t *testing.T) {
		cfg := Config{
			APIKey: "key-g",
		}
		got, err := cfg.toClientConfig()
		if err != nil {
			t.Fatalf("toClientConfig() error = %v", err)
		}
		if got.APIKey != "key-g" || got.Backend != genai.BackendGeminiAPI {
			t.Errorf("toClientConfig() produced invalid Gemini config: %+v", got)
		}
	})
}

func TestConfig_retryParams(t *testing.T) {
	t.Run("デフォルト値が適用されること", func(t *testing.T) {
		cfg := Config{}
		got := cfg.retryParams()
		if got.MaxRetries != clampToUint(DefaultMaxRetries) {
			t.Errorf("MaxRetries = %v, want %v", got.MaxRetries, DefaultMaxRetries)
		}
		if got.InitialInterval != DefaultInitialDelay || got.MaxInterval != DefaultMaxDelay {
			t.Errorf("インターバルのデフォルトが適用されていません: %+v", got)
		}
	})

	t.Run("設定値で上書きされること", func(t *testing.T) {
		cfg := Config{
			MaxRetries:   Ptr[uint64](5),
			InitialDelay: 10 * time.Second,
			MaxDelay:     60 * time.Second,
		}
		got := cfg.retryParams()
		if got.MaxRetries != 5 || got.InitialInterval != 10*time.Second || got.MaxInterval != 60*time.Second {
			t.Errorf("設定が正しく適用されていません: %+v", got)
		}
	})

	// 0 は「再試行しない」で、未設定の既定値へは倒しません。
	// MaxRetries が値型だったころは構造体のゼロ値と区別できず、これを書く手段が
	// そもそもありませんでした（0 と書いても DefaultMaxRetries が使われていた）。
	t.Run("明示した 0 は再試行しないこと", func(t *testing.T) {
		cfg := Config{MaxRetries: Ptr[uint64](0)}
		got := cfg.retryParams()
		if got.MaxRetries != 0 {
			t.Errorf("MaxRetries = %v, want 0", got.MaxRetries)
		}
	})

	// nil のときだけ既定値へ倒すこと。ゼロ値の Config が既定回数で再試行する
	// 従来の挙動は変えていません。
	t.Run("nil は既定値へ倒すこと", func(t *testing.T) {
		cfg := Config{MaxRetries: nil}
		got := cfg.retryParams()
		if got.MaxRetries != clampToUint(DefaultMaxRetries) {
			t.Errorf("MaxRetries = %v, want %v", got.MaxRetries, DefaultMaxRetries)
		}
	})

	t.Run("Option 列に変換できること", func(t *testing.T) {
		opts := Config{MaxRetries: Ptr[uint64](5)}.buildRetryOptions()
		if len(opts) != 3 {
			t.Errorf("Option の数が不正です: %d", len(opts))
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

// TestToClientConfigAttachesCredentialsToSuppliedHTTPClient は、HTTPClient を指定しても
// Vertex AI の認証が効くことを検証します。
//
// genai は ClientConfig.HTTPClient が非 nil だと ADC の検出をスキップし、渡された
// クライアントを認証ヘッダ無しで使います。そのため素の &http.Client{Timeout: ...} を
// 渡すと全リクエストが 401 (CREDENTIALS_MISSING) になります。実際にこれで本番が
// 停止したため、Transport が差し替えられている（＝認証が付いている）ことを確認します。
func TestToClientConfigAttachesCredentialsToSuppliedHTTPClient(t *testing.T) {
	skipWithoutGCPCredentials(t)

	supplied := &http.Client{Timeout: 42 * time.Second}
	cfg := Config{ProjectID: "p", LocationID: "us-central1", HTTPClient: supplied}

	got, err := cfg.toClientConfig()
	if err != nil {
		t.Fatalf("toClientConfig() error = %v", err)
	}
	if got.HTTPClient == nil {
		t.Fatal("HTTPClient = nil, want the supplied client")
	}
	if got.HTTPClient.Timeout != 42*time.Second {
		t.Errorf("Timeout = %v, want the supplied value to survive", got.HTTPClient.Timeout)
	}
	if got.HTTPClient.Transport == nil {
		t.Error("Transport = nil, want the authorization middleware attached")
	}
	// 呼び出し側のインスタンスは書き換えない（他所で使い回されている可能性がある）。
	if supplied.Transport != nil {
		t.Error("the caller's http.Client was mutated; it should have been copied")
	}
	if got.HTTPClient == supplied {
		t.Error("HTTPClient is the caller's instance; it should have been copied")
	}
}

// TestToClientConfigLeavesGeminiAPIClientAlone は、Gemini API バックエンドでは渡された
// クライアントに認証情報を付けないことを検証します。API キーはヘッダで送られるため
// Transport に依存せず、ADC を探しに行く必要もありません。
func TestToClientConfigLeavesGeminiAPIClientAlone(t *testing.T) {
	supplied := &http.Client{Timeout: 42 * time.Second}
	cfg := Config{APIKey: "test-key", HTTPClient: supplied}

	got, err := cfg.toClientConfig()
	if err != nil {
		t.Fatalf("toClientConfig() error = %v", err)
	}
	if got.HTTPClient.Timeout != 42*time.Second {
		t.Errorf("Timeout = %v", got.HTTPClient.Timeout)
	}
	if got.HTTPClient.Transport != nil {
		t.Error("Transport was replaced on the Gemini API backend; API keys travel as a header")
	}
}
