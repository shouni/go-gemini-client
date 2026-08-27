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

func TestConfig_retryOptions(t *testing.T) {
	t.Run("デフォルト値が適用されること", func(t *testing.T) {
		got := Config{}.retryOptions()
		if got == nil {
			t.Fatal("retryOptions() = nil, want 既定値つきの設定")
		}
		if *got.Attempts != int32(DefaultMaxRetries)+1 {
			t.Errorf("Attempts = %v, want %v", *got.Attempts, DefaultMaxRetries+1)
		}
		if *got.InitialDelay != DefaultInitialDelay.Seconds() || *got.MaxDelay != DefaultMaxDelay.Seconds() {
			t.Errorf("待ち時間のデフォルトが適用されていません: %+v", got)
		}
	})

	t.Run("設定値で上書きされること", func(t *testing.T) {
		got := Config{
			MaxRetries:   5,
			InitialDelay: 10 * time.Second,
			MaxDelay:     60 * time.Second,
		}.retryOptions()
		if *got.Attempts != 6 {
			t.Errorf("Attempts = %v, want 6 (初回 + リトライ5回)", *got.Attempts)
		}
		if *got.InitialDelay != 10 || *got.MaxDelay != 60 {
			t.Errorf("待ち時間が正しく適用されていません: %+v", got)
		}
	})

	// SDK の既定ジッタ（U(0, 1秒) の加算）は InitialDelay が数十秒だとほぼ効かないため、
	// 初期間隔に比例した幅を明示します。
	t.Run("ジッタが初期間隔に比例すること", func(t *testing.T) {
		got := Config{InitialDelay: 60 * time.Second}.retryOptions()
		if *got.Jitter != 30 {
			t.Errorf("Jitter = %v, want 30", *got.Jitter)
		}
	})

	// nil は SDK 側で「1 回だけ実行」を意味します。値型の MaxRetries では
	// ゼロ値と「リトライしない」を区別できないため、専用のフラグで表します。
	t.Run("DisableRetry で nil になること", func(t *testing.T) {
		if got := (Config{DisableRetry: true}).retryOptions(); got != nil {
			t.Errorf("retryOptions() = %+v, want nil", got)
		}
	})

	t.Run("MaxRetries の 0 は未設定として既定値へ倒すこと", func(t *testing.T) {
		got := Config{MaxRetries: 0}.retryOptions()
		if *got.Attempts != int32(DefaultMaxRetries)+1 {
			t.Errorf("Attempts = %v, want %v", *got.Attempts, DefaultMaxRetries+1)
		}
	})

	t.Run("クライアント設定に載ること", func(t *testing.T) {
		cc, err := Config{APIKey: "key", MaxRetries: 2}.toClientConfig()
		if err != nil {
			t.Fatalf("toClientConfig() error = %v", err)
		}
		if cc.HTTPOptions.RetryOptions == nil || *cc.HTTPOptions.RetryOptions.Attempts != 3 {
			t.Errorf("HTTPOptions.RetryOptions = %+v, want Attempts=3", cc.HTTPOptions.RetryOptions)
		}
	})
}

func TestNoRetryHTTPOptions(t *testing.T) {
	// リクエスト側の RetryOptions は非 nil のときだけクライアント設定を上書きするため、
	// 打ち消しには nil ではなく Attempts=1 を渡す必要があります。
	got := noRetryHTTPOptions()
	if got.RetryOptions == nil || *got.RetryOptions.Attempts != 1 {
		t.Errorf("noRetryHTTPOptions() = %+v, want Attempts=1", got.RetryOptions)
	}
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
