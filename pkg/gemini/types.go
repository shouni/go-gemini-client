package gemini

import (
	"errors"
	"time"

	"github.com/shouni/netarmor/retry"
	"google.golang.org/genai"
)

const (
	DefaultMaxRetries   uint64        = 1
	DefaultInitialDelay time.Duration = 30 * time.Second
	DefaultMaxDelay     time.Duration = 120 * time.Second

	DefaultTemperature    float32 = 0.7
	DefaultTopP           float32 = 0.95
	DefaultCandidateCount int32   = 1

	// File API
	PollingInterval     = 2 * time.Second
	PollingTimeout      = 60 * time.Second
	AsyncCleanupTimeout = 15 * time.Second
)

// Config は初期化用の設定です。
// Vertex AI を使用する場合は ProjectID と LocationID を指定してください。
// Gemini API (Google AI Studio) を使用する場合は APIKey を指定してください。
type Config struct {
	APIKey       string
	ProjectID    string // Vertex AI: Google Cloud Project ID
	LocationID   string // Vertex AI: Location (e.g., "us-central1")
	Temperature  *float32
	MaxRetries   uint64
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

// Client は Gemini SDK をラップしたメイン構造体です。
type Client struct {
	client      *genai.Client
	temperature float32
	retryConfig retry.Config
	backend     genai.Backend
}

// PersonGeneration は人物生成の許可設定を表すカスタム型です。
type PersonGeneration string

const (
	// PersonGenerationUnspecified は設定を省略し、APIのデフォルトに委ねます。
	PersonGenerationUnspecified PersonGeneration = ""
	// PersonGenerationAllowAll はすべての人物生成を許可します（キャラクター生成に推奨）。
	PersonGenerationAllowAll PersonGeneration = "ALLOW_ALL"
	// PersonGenerationAllowAdult は成人のみの生成を許可します（SDKデフォルト）。
	PersonGenerationAllowAdult PersonGeneration = "ALLOW_ADULT"
	// PersonGenerationDontAllow は人物の生成を許可しません。
	PersonGenerationDontAllow PersonGeneration = "DONT_ALLOW"
)

// GenerateOptions は各生成リクエストごとのオプションです。
type GenerateOptions struct {
	SystemPrompt   string
	Temperature    *float32
	TopP           *float32
	CandidateCount *int32
	// 画像生成 (Nano Banana / Imagen) 特有のパラメータ
	AspectRatio      string
	ImageSize        string
	Seed             *int64
	PersonGeneration PersonGeneration
	SafetySettings   []*genai.SafetySetting
}

// Response は生成結果のラッパーです。
type Response struct {
	Text        string
	Images      [][]byte // 生成画像 (InlineData) を保持します
	RawResponse *genai.GenerateContentResponse
}

// 堅牢なエラーハンドリングのためのパッケージレベルのセンチネルエラー。
var (
	// 初期化時のエラー
	ErrConfigRequired         = errors.New("APIKey または ProjectID/LocationID のいずれかが必須です")
	ErrExclusiveConfig        = errors.New("ProjectID/LocationID と APIKey は排他的に設定してください")
	ErrIncompleteVertexConfig = errors.New("Vertex AIを使用する場合、ProjectIDとLocationIDの両方を設定してください")

	// 設定・バリデーションのエラー
	ErrInvalidTemperature = errors.New("温度設定（Temperature）は 0.0 から 2.0 の間である必要があります")
	ErrEmptyPrompt        = errors.New("プロンプトを空にすることはできません")
)

// IsVertexAI ProjectIDおよびLocationIDのセットを確認し、Vertex AIの設定が有効であるかをチェックします。
func (c Config) IsVertexAI() bool {
	return c.ProjectID != "" && c.LocationID != ""
}

// IsGeminiAPI APIKeyの有無を検証し、Gemini APIを利用するための設定が有効であるかを確認します。
func (c Config) IsGeminiAPI() bool {
	return c.APIKey != ""
}

// IsIncompleteVertex ProjectIDまたはLocationIDの有無を確認し、Vertex AIの設定漏れがないかを検証します。
func (c Config) IsIncompleteVertex() bool {
	hasAny := c.ProjectID != "" || c.LocationID != ""
	return hasAny && !c.IsVertexAI()
}

func (o *GenerateOptions) HasImageConfig() bool {
	if o == nil {
		return false
	}
	return o.AspectRatio != "" || o.ImageSize != "" || o.PersonGeneration != PersonGenerationUnspecified
}
