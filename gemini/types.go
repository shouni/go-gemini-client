package gemini

import (
	"errors"
	"time"

	"google.golang.org/genai"
)

const (
	// DefaultMaxRetries は、リトライ回数が未設定の場合に使用されるデフォルト値です。
	DefaultMaxRetries uint64 = 1
	// DefaultInitialDelay は、初期リトライ間隔が未設定の場合に使用されるデフォルト値です。
	DefaultInitialDelay time.Duration = 30 * time.Second
	// DefaultMaxDelay は、最大リトライ間隔が未設定の場合に使用されるデフォルト値です。
	DefaultMaxDelay time.Duration = 120 * time.Second

	// PollingInterval は、File API のアップロード完了確認のポーリング間隔です。
	PollingInterval = 2 * time.Second
	// PollingTimeout は、File API のアップロード完了確認のタイムアウトです。
	PollingTimeout = 60 * time.Second
	// AsyncCleanupTimeout は、非同期クリーンアップ処理のタイムアウトです。
	AsyncCleanupTimeout = 15 * time.Second
)

var (
	// ErrEmptyPrompt は、プロンプトが空の場合に返されます。
	ErrEmptyPrompt = errors.New("プロンプトを空にすることはできません")
	// ErrEmptyModelName は、モデル名が空の場合に返されます。
	ErrEmptyModelName = errors.New("モデル名を空にすることはできません")
	// ErrEmptyParts は、生成パーツが空の場合に返されます。
	ErrEmptyParts = errors.New("生成パーツを空にすることはできません")
	// ErrInvalidPart は、生成パーツに nil が含まれる場合に返されます。
	ErrInvalidPart = errors.New("生成パーツに nil を含めることはできません")
	// ErrInvalidSeed は、Seed が int32 の範囲外の場合に返されます。
	ErrInvalidSeed = errors.New("seed は int32 の範囲内である必要があります")
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
	SystemPrompt string
	// 画像生成 (Nano Banana / Imagen) 特有のパラメータ
	AspectRatio      string
	ImageSize        string
	Seed             *int64
	PersonGeneration PersonGeneration
	SafetySettings   []*genai.SafetySetting
	ResponseMIMEType string
	// ResponseSchema は構造化出力のスキーマです。ResponseMIMEType "application/json" と
	// 併用すると、モデル出力が文法レベルでスキーマに制約され、JSON 以外の
	// 余計なテキストが混入しなくなります。
	ResponseSchema *genai.Schema
}

// Response は生成結果のラッパーです。
type Response struct {
	Text        string
	Images      [][]byte // 生成画像 (InlineData) を保持します
	Audios      [][]byte // Lyria 3 等の音声データ
	Usage       *TokenUsage
	RawResponse *genai.GenerateContentResponse
}

// TokenUsage は生成レスポンスのトークン使用量です。
type TokenUsage struct {
	PromptTokenCount     int32
	CandidatesTokenCount int32
	TotalTokenCount      int32
}

// HasImageConfig は、画像生成特有のパラメータが1つでも設定されているかを判定します。
func (o *GenerateOptions) HasImageConfig() bool {
	if o == nil {
		return false
	}
	return o.AspectRatio != "" || o.ImageSize != "" || o.PersonGeneration != PersonGenerationUnspecified
}

// NewSafetySettings は、標準的な4つのハームカテゴリ（暴力・ヘイト・性的表現・危険行為）
// すべてに同一の閾値を適用した SafetySetting のスライスを返します。
// 閾値をバックエンドや用途に応じてどう選ぶかは呼び出し側の判断に委ねます。
func NewSafetySettings(threshold genai.HarmBlockThreshold) []*genai.SafetySetting {
	return []*genai.SafetySetting{
		{Category: genai.HarmCategoryHarassment, Threshold: threshold},
		{Category: genai.HarmCategoryHateSpeech, Threshold: threshold},
		{Category: genai.HarmCategorySexuallyExplicit, Threshold: threshold},
		{Category: genai.HarmCategoryDangerousContent, Threshold: threshold},
	}
}
