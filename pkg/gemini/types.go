package gemini

import (
	"errors"
	"time"

	"github.com/shouni/go-utils/retry"
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

// Config は初期化用の設定
type Config struct {
	APIKey       string
	Temperature  *float32
	MaxRetries   uint64
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

// Client は Gemini SDK をラップしたメイン構造体
type Client struct {
	client      *genai.Client
	temperature float32
	retryConfig retry.Config
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

// 堅牢なエラーハンドリングのためのパッケージレベルのセンチネルエラー。
var (
	// 初期化時のエラー
	ErrAPIKeyRequired = errors.New("APIキーは必須です")

	// 設定・バリデーションのエラー
	ErrInvalidTemperature = errors.New("温度設定（Temperature）は 0.0 から 1.0 の間である必要があります")
	ErrEmptyPrompt        = errors.New("プロンプトを空にすることはできません")
)
