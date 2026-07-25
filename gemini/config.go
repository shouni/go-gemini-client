package gemini

import (
	"errors"
	"net/http"
	"time"

	"github.com/shouni/netarmor/retry"
	"google.golang.org/genai"
)

var (
	// ErrConfigRequired は、APIKey と ProjectID/LocationID のいずれも設定されていない場合に返されます。
	ErrConfigRequired = errors.New("APIKey または ProjectID/LocationID のいずれかが必須です")
	// ErrExclusiveConfig は、ProjectID/LocationID と APIKey が同時に設定された場合に返されます。
	ErrExclusiveConfig = errors.New("ProjectID/LocationID と APIKey は排他的に設定してください")
	// ErrIncompleteVertexConfig は、ProjectID と LocationID の一方のみが設定された場合に返されます。
	ErrIncompleteVertexConfig = errors.New("vertex AIを使用する場合、ProjectIDとLocationIDの両方を設定してください")
)

// Config は初期化用の設定です。
// Vertex AI を使用する場合は ProjectID と LocationID を指定してください。
// Gemini API (Google AI Studio) を使用する場合は APIKey を指定してください。
type Config struct {
	APIKey              string
	ProjectID           string // Vertex AI: Google Cloud Project ID
	LocationID          string // Vertex AI: Location (e.g., "us-central1")
	MaxRetries          uint64
	InitialDelay        time.Duration
	MaxDelay            time.Duration
	FilePollingInterval time.Duration
	FilePollingTimeout  time.Duration

	// HTTPClient は genai SDK が使用する HTTP クライアントを差し替えます。
	// nil の場合は SDK のデフォルトが使われます。
	//
	// タイムアウトやプロキシを制御したい場合、あるいは SSRF 対策済みの
	// クライアントを使いたい場合に指定します。
	//
	//	cfg.HTTPClient = securenet.NewSafeHTTPClient(60 * time.Second)
	HTTPClient *http.Client

	// OnRetry はリトライ直前に呼び出されます。nil の場合は何もしません。
	// 429 が続いた場合の可視化などに使用します。
	//
	//	cfg.OnRetry = func(err error, attempt uint, next time.Duration) {
	//	    slog.Warn("gemini retry", "attempt", attempt, "next", next, "err", err)
	//	}
	OnRetry retry.NotifyFunc
}

// isVertexAI ProjectIDおよびLocationIDのセットを確認し、Vertex AIの設定が有効であるかをチェックします。
func (c Config) isVertexAI() bool {
	return c.ProjectID != "" && c.LocationID != ""
}

// isGeminiAPI APIKeyの有無を検証し、Gemini APIを利用するための設定が有効であるかを確認します。
func (c Config) isGeminiAPI() bool {
	return c.APIKey != ""
}

// isIncompleteVertex ProjectIDまたはLocationIDの有無を確認し、Vertex AIの設定漏れがないかを検証します。
func (c Config) isIncompleteVertex() bool {
	hasAny := c.ProjectID != "" || c.LocationID != ""
	return hasAny && !c.isVertexAI()
}

// validate は設定内容が正しいか、排他制御や値の範囲をチェックします。
func (c Config) validate() error {
	// 1. 排他制御
	if (c.isVertexAI() || c.isIncompleteVertex()) && c.isGeminiAPI() {
		return ErrExclusiveConfig
	}

	// 2. 完全性チェック
	if c.isIncompleteVertex() {
		return ErrIncompleteVertexConfig
	}

	// 3. 必須チェック
	if !c.isVertexAI() && !c.isGeminiAPI() {
		return ErrConfigRequired
	}

	return nil
}

// toClientConfig Config を genai.ClientConfig に変換します。
func (c Config) toClientConfig() *genai.ClientConfig {
	cc := &genai.ClientConfig{HTTPClient: c.HTTPClient}
	if c.isVertexAI() {
		cc.Project = c.ProjectID
		cc.Location = c.LocationID
		cc.Backend = genai.BackendVertexAI
	} else {
		cc.APIKey = c.APIKey
		cc.Backend = genai.BackendGeminiAPI
	}
	return cc
}

// orDefault は v が正の値であればそれを、そうでなければ def を返します。
func orDefault(v, def time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return def
}

// retryParams は Config から解決済みのリトライ設定値です。
// retry.Option は不透明な関数値でテストから検査できないため、
// 「どの値が採用されたか」を検証可能にする中間表現として保持しています。
type retryParams struct {
	MaxRetries      uint
	InitialInterval time.Duration
	MaxInterval     time.Duration
}

// retryParams は未設定項目をデフォルトで補完したリトライ設定を返します。
//
// MaxRetries が 0 の場合は DefaultMaxRetries にフォールバックします
// （netarmor の retry.WithMaxRetries は 0 を「リトライしない」と解釈するため、
// この分岐がないと未設定時にリトライが行われなくなります）。
func (c Config) retryParams() retryParams {
	maxRetries := c.MaxRetries
	if maxRetries == 0 {
		maxRetries = DefaultMaxRetries
	}

	return retryParams{
		MaxRetries:      clampToUint(maxRetries),
		InitialInterval: orDefault(c.InitialDelay, DefaultInitialDelay),
		MaxInterval:     orDefault(c.MaxDelay, DefaultMaxDelay),
	}
}

// options は netarmor の retry.Option 列に変換します。
func (p retryParams) options() []retry.Option {
	return []retry.Option{
		retry.WithMaxRetries(p.MaxRetries),
		retry.WithInitialInterval(p.InitialInterval),
		retry.WithMaxInterval(p.MaxInterval),
	}
}

// buildRetryOptions は設定から netarmor の retry.Option 列を構築します。
func (c Config) buildRetryOptions() []retry.Option {
	opts := c.retryParams().options()
	if c.OnRetry != nil {
		opts = append(opts, retry.WithNotify(c.OnRetry))
	}
	return opts
}

// clampToUint は uint64 -> uint の変換を飽和させます（32bit 環境での切り詰め防止）。
func clampToUint(v uint64) uint {
	if v > uint64(^uint(0)) {
		return ^uint(0)
	}
	return uint(v)
}

func (c Config) getFilePollingInterval() time.Duration {
	return orDefault(c.FilePollingInterval, PollingInterval)
}

func (c Config) getFilePollingTimeout() time.Duration {
	return orDefault(c.FilePollingTimeout, PollingTimeout)
}
