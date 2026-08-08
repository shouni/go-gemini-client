package gemini

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/shouni/netarmor/retry"
	"google.golang.org/genai"
)

var (
	// ErrConfigRequired は、APIKey と ProjectID/LocationID のいずれも設定されていない場合に返されます。
	ErrConfigRequired = errors.New("gemini: either APIKey or ProjectID/LocationID is required")
	// ErrExclusiveConfig は、ProjectID/LocationID と APIKey が同時に設定された場合に返されます。
	ErrExclusiveConfig = errors.New("gemini: ProjectID/LocationID and APIKey are mutually exclusive")
	// ErrIncompleteVertexConfig は、ProjectID と LocationID の一方のみが設定された場合に返されます。
	ErrIncompleteVertexConfig = errors.New("gemini: Vertex AI requires both ProjectID and LocationID")
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

	// RequestTimeout は、生成呼び出し1回（リトライを含む）の上限時間です。
	// 0 は無制限で、呼び出し側の context の期限にのみ従います。
	// File API のアップロード待ちとポーリングには適用されません
	// （それぞれ FilePollingTimeout と veo 側の設定が受け持ちます）。
	RequestTimeout time.Duration

	// AsyncCleanupTimeout は、アップロード後処理の失敗時にバックグラウンドで行う
	// ファイル削除の上限時間です。0 はデフォルト（15秒）です。
	AsyncCleanupTimeout time.Duration

	// Logger は、このクライアントが出すログの出力先です。nil の場合は
	// slog.Default() を使います。ジョブ ID などの属性を付けたロガーを渡すと、
	// ライブラリ内部のログにもその属性が乗ります。
	Logger *slog.Logger

	// HTTPClient は genai SDK が使用する HTTP クライアントを差し替えます。
	// nil の場合は SDK のデフォルトが使われます。
	//
	// タイムアウトやプロキシを制御したい場合、あるいは SSRF 対策済みの
	// クライアントを使いたい場合に指定します。
	//
	//	cfg.HTTPClient = securenet.NewSafeHTTPClient(60 * time.Second)
	//
	// 認証は気にしなくて構いません。genai は HTTPClient を渡されると ADC の検出を
	// スキップして認証ヘッダ無しで送ってしまいますが、toClientConfig が Vertex AI では
	// 認証情報を付け直します。渡したインスタンス自体は書き換えず、複製を使います。
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
//
// HTTPClient が指定されている場合、Vertex AI では認証情報の付与も行います。genai は
// ClientConfig.HTTPClient が非 nil だと ADC の検出そのものをスキップし、渡された
// クライアントを認証ヘッダ無しで使うため、これが無いと全リクエストが 401
// （CREDENTIALS_MISSING）になります。つまり素の &http.Client{Timeout: ...} を渡すと、
// タイムアウトを設定したつもりで認証を捨てることになります。
func (c Config) toClientConfig() (*genai.ClientConfig, error) {
	cc := &genai.ClientConfig{}
	if c.isVertexAI() {
		cc.Project = c.ProjectID
		cc.Location = c.LocationID
		cc.Backend = genai.BackendVertexAI
	} else {
		cc.APIKey = c.APIKey
		cc.Backend = genai.BackendGeminiAPI
	}

	if c.HTTPClient == nil {
		return cc, nil
	}
	// UseDefaultCredentials は渡されたクライアントの Transport を書き換えるため、
	// 呼び出し側が持っているインスタンスには触らないよう浅いコピーへ差し替える。
	// Timeout などの設定は引き継がれる。
	clone := *c.HTTPClient
	cc.HTTPClient = &clone

	// Gemini API バックエンドの認証は API キーのヘッダ付与で、Transport には依存しない。
	if cc.Backend != genai.BackendVertexAI {
		return cc, nil
	}
	if err := cc.UseDefaultCredentials(); err != nil {
		return nil, fmt.Errorf("gemini: 指定された HTTPClient への認証情報の付与に失敗しました: %w", err)
	}
	return cc, nil
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

func (c Config) getAsyncCleanupTimeout() time.Duration {
	return orDefault(c.AsyncCleanupTimeout, AsyncCleanupTimeout)
}

func (c Config) getLogger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}
