package gemini

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

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
	APIKey     string
	ProjectID  string // Vertex AI: Google Cloud Project ID
	LocationID string // Vertex AI: Location (e.g., "us-central1")

	// MaxRetries は、1 回の呼び出しで許すリトライの回数です（初回実行は含みません）。
	// 0 は未設定で、DefaultMaxRetries を使います。リトライを止めたい場合は
	// DisableRetry を立ててください。
	MaxRetries uint

	// DisableRetry はリトライを完全に無効にし、1 回だけ実行します。
	// 成功のたびに副作用が生まれる呼び出しで、応答を取りこぼした際の再送が
	// 二重実行になる場合に使います。
	DisableRetry bool

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
}

// isVertexAI は、Vertex AI を使う設定（ProjectID と LocationID の両方）が揃っているかを返します。
func (c Config) isVertexAI() bool {
	return c.ProjectID != "" && c.LocationID != ""
}

// isGeminiAPI は、Gemini API を使う設定（APIKey）があるかを返します。
func (c Config) isGeminiAPI() bool {
	return c.APIKey != ""
}

// isIncompleteVertex は、Vertex AI の設定が片方だけ埋まっているかを返します。
// 両方空（Gemini API を使う）でも両方揃っている場合でもなく、書きかけの状態です。
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
	cc.HTTPOptions.RetryOptions = c.retryOptions()
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

// retryOptions は Config を genai SDK 内蔵リトライの設定へ変換します。
// nil を返すと SDK はリトライせず 1 回だけ実行します。
//
// リトライを SDK に任せているのは、genai が HTTP 層で 408/429/5xx と通信エラーを
// 判定するためです。自前で包んでいたころは判定表を写し取る必要があり、SDK 側が
// 対象を増やしたときにこちらだけ古い一覧を持ち続けていました。
func (c Config) retryOptions() *genai.HTTPRetryOptions {
	if c.DisableRetry {
		return nil
	}

	maxRetries := c.MaxRetries
	if maxRetries == 0 {
		maxRetries = DefaultMaxRetries
	}
	initialDelay := orDefault(c.InitialDelay, DefaultInitialDelay)
	maxDelay := orDefault(c.MaxDelay, DefaultMaxDelay)

	return &genai.HTTPRetryOptions{
		Attempts:     new(attemptsFrom(maxRetries)),
		InitialDelay: new(initialDelay.Seconds()),
		MaxDelay:     new(maxDelay.Seconds()),
		// SDK の既定ジッタは U(0, 1秒) の加算で、InitialDelay が数十秒の設定では
		// ほぼ効きません。並列の生成が同じ 429 で弾かれたとき、散らすのはジッタ
		// だけです（callguard の発射間隔はリトライには掛からない）。待ち時間に対して
		// 幅を持たせるため、初期間隔の半分を指定します。
		Jitter: new(initialDelay.Seconds() / 2),
	}
}

// noRetryHTTPOptions は、クライアントのリトライ設定をリクエスト単位で打ち消します。
// ポーリングのように呼び出し側が間隔とタイムアウトを持っている経路で使います。
//
// nil ではなく「1 回だけ」を明示するのは、genai がリクエスト側の RetryOptions を
// 非 nil のときだけクライアント設定に上書き適用するためです。
func noRetryHTTPOptions() *genai.HTTPOptions {
	return &genai.HTTPOptions{RetryOptions: &genai.HTTPRetryOptions{Attempts: new(int32(1))}}
}

// attemptsFrom はリトライ回数を genai の総試行回数（初回を含む）へ変換します。
// int32 に収まらない指定は飽和させます。
func attemptsFrom(maxRetries uint) int32 {
	if uint64(maxRetries) >= math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(maxRetries) + 1
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
