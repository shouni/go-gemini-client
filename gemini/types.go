package gemini

import (
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
//
// ポインタ型のフィールドは「未設定」と「明示的なゼロ値」を区別するためのものです。
// 例えば Temperature は 0（決定的な出力）が意味を持つ値なので、nil のときだけ
// SDK のデフォルトに委ねます。設定には Ptr ヘルパーが使えます。
//
//	opts := gemini.GenerateOptions{Temperature: gemini.Ptr[float32](0)}
type GenerateOptions struct {
	SystemPrompt string

	// --- サンプリングパラメータ ---

	// Temperature は出力のランダム性です（0 で最も決定的）。nil で SDK デフォルト。
	Temperature *float32
	// TopP は核サンプリングの閾値です。nil で SDK デフォルト。
	TopP *float32
	// TopK は上位 K 個からのサンプリング数です。nil で SDK デフォルト。
	TopK *float32
	// MaxOutputTokens は生成する最大トークン数です。0 で SDK デフォルト。
	MaxOutputTokens int32
	// StopSequences は生成を打ち切る文字列のリストです。
	StopSequences []string

	// --- 思考 (Gemini 2.5 以降) ---

	// ThinkingBudget は思考に使うトークン数の上限です。
	// 0 を明示すると思考を無効化し、レイテンシとコストを抑えられます。
	// nil ではモデルのデフォルトに委ねます。
	//
	// 有効なトークン数の範囲はモデルごとに異なります。モデルを跨いで使う場合は
	// ThinkingLevel のほうが移植性があります。
	ThinkingBudget *int32
	// ThinkingLevel は思考量を段階で指定します（MINIMAL / LOW / MEDIUM / HIGH）。
	// ThinkingBudget のトークン数指定に対し、モデル非依存で扱えるのが利点です。
	//
	// ThinkingBudget と ThinkingLevel は排他的な指定方法です。
	// 両方を設定した場合は ThinkingLevel を優先し、ThinkingBudget は送信しません。
	ThinkingLevel genai.ThinkingLevel
	// IncludeThoughts を true にすると思考サマリが返され、Response.Thoughts で取得できます。
	IncludeThoughts bool

	// --- 画像生成 (Nano Banana / Imagen) 特有のパラメータ ---

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
	// ResponseJSONSchema は、標準的な JSON Schema による構造化出力の指定です。
	// $ref を含むような複雑なスキーマで ResponseSchema がうまく機能しない場合の
	// 代替として使用します。
	//
	// ResponseSchema と ResponseJSONSchema は排他的な指定方法です。
	// 両方を設定した場合は ResponseJSONSchema を優先し、ResponseSchema は送信しません。
	ResponseJSONSchema any
}

// Ptr は任意の値へのポインタを返すヘルパーです。
// GenerateOptions のポインタ型フィールドにリテラルを設定する際に使用します。
func Ptr[T any](v T) *T { return &v }

// Response は生成結果のラッパーです。
type Response struct {
	Text   string
	Images [][]byte // 生成画像 (InlineData) を保持します
	Audios [][]byte // Lyria 3 等の音声データ
	// Thoughts は思考サマリです。GenerateOptions.IncludeThoughts が true で、
	// かつモデルが思考サマリを返した場合にのみ設定されます。
	// Text には含まれません。
	Thoughts    string
	Usage       *TokenUsage
	RawResponse *genai.GenerateContentResponse
}

// TokenUsage は生成レスポンスのトークン使用量です。
type TokenUsage struct {
	PromptTokenCount     int32
	CandidatesTokenCount int32
	TotalTokenCount      int32
	// ThoughtsTokenCount は思考に消費されたトークン数です。
	// 課金対象になるため、思考機能を使う場合はこの値を監視してください。
	ThoughtsTokenCount int32
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
