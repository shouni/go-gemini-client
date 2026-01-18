package gemini

import (
	"context"
	"time"

	"github.com/shouni/go-utils/retry"
	"google.golang.org/genai"
)

const (
	DefaultTemperature  float32 = 0.7
	DefaultMaxRetries           = 1
	DefaultInitialDelay         = 30 * time.Second
	DefaultMaxDelay             = 120 * time.Second

	DefaultTopP           float32 = 0.95
	DefaultCandidateCount int32   = 1

	// File API
	PollingInterval     = 2 * time.Second
	PollingTimeout      = 60 * time.Second
	AsyncCleanupTimeout = 15 * time.Second
)

// Config は初期化用の設定
type Config struct {
	APIKey       string
	Temperature  *float32 // 0を許容するためポインタが安全
	MaxRetries   uint64
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

// Client は Gemini SDK をラップしたメイン構造体
type Client struct {
	client      *genai.Client
	temperature float32      // NewClient で確定させた値を保持
	retryConfig retry.Config // 確定させたリトライ設定を保持
}

// GenerateOptions は各生成リクエストごとのオプション
type GenerateOptions struct {
	SystemPrompt   string
	Temperature    *float32
	TopP           *float32
	CandidateCount *int32
	// 画像生成 (Nano Banana) 特有のパラメータ
	AspectRatio    string
	Seed           *int64
	SafetySettings []*genai.SafetySetting
}

// Response は生成結果のラッパー
type Response struct {
	Text        string
	Images      [][]byte // 生成画像（将来的な拡張用）
	RawResponse *genai.GenerateContentResponse
}

// GenerativeModel インターフェース
// Client がこれを満たすように実装します
type GenerativeModel interface {
	GenerateContent(ctx context.Context, modelName string, prompt string) (*Response, error)
	GenerateWithParts(ctx context.Context, modelName string, parts []*genai.Part, opts GenerateOptions) (*Response, error)
	UploadFile(ctx context.Context, data []byte, mimeType, displayName string) (string, string, error)
	DeleteFile(ctx context.Context, fileName string) error
}
