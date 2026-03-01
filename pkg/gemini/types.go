package gemini

import (
	"time"

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

func (o *GenerateOptions) HasImageConfig() bool {
	if o == nil {
		return false
	}
	return o.AspectRatio != "" || o.ImageSize != "" || o.PersonGeneration != PersonGenerationUnspecified
}
