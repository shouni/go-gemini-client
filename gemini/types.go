package gemini

import (
	"fmt"
	"math"
	"time"

	"google.golang.org/genai"
)

const (
	// DefaultMaxRetries は、リトライ回数が未設定の場合に使用されるデフォルト値です。
	DefaultMaxRetries uint = 1
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
	// PersonGenerationUnspecified は設定を省略し、API のデフォルトに委ねます。
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
// SDK のデフォルトに委ねます。設定には new(式) を使います。
//
//	opts := gemini.GenerateOptions{Temperature: new(float32(0))}
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

	// AspectRatio は生成画像の縦横比です（例: "16:9"）。空で API デフォルト。
	AspectRatio string
	// ImageSize は生成画像の解像度指定です（例: "1K"）。空で API デフォルト。
	ImageSize string
	// Seed は再現性のためのシード値です。int32 の範囲外を指定すると
	// ErrInvalidSeed になります。nil で API に委ねます。
	Seed *int64
	// PersonGeneration は人物生成の許可設定です。Vertex AI でのみ送信され、
	// Gemini API バックエンドでは無視されます。
	PersonGeneration PersonGeneration

	// SafetySettings は安全フィルタの設定です。NewSafetySettings で組み立てられます。
	SafetySettings []*genai.SafetySetting
	// ResponseMIMEType は期待するレスポンスの MIME type です
	// （"application/json" / "image/png" / "audio/wav" など）。
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

// Response は生成結果のラッパーです。
type Response struct {
	Text   string
	Images [][]byte // 生成画像 (InlineData) を保持します
	Audios [][]byte // Lyria 3 等の音声データ
	// Attachments は返却されたインラインデータを MIME type 付きで保持します。
	//
	// Images / Audios はバイト列だけなので、保存時の拡張子や Content-Type を決める
	// 必要がある呼び出し側のために、型を保ったまま取り出せる形を用意しています。
	// 順序は API の返却順です。
	Attachments []Attachment
	// Thoughts は思考サマリです。GenerateOptions.IncludeThoughts が true で、
	// かつモデルが思考サマリを返した場合にのみ設定されます。
	// Text には含まれません。
	Thoughts string
	Usage    *TokenUsage
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

// ThinkingLevel は、思考量の段階指定です（genai.ThinkingLevel の別名）。
type ThinkingLevel = genai.ThinkingLevel

// 思考量の段階です。
const (
	// ThinkingUnspecified はモデルのデフォルトに委ねます。
	ThinkingUnspecified ThinkingLevel = genai.ThinkingLevelUnspecified
	// ThinkingMinimal は思考をほぼ行わず、レイテンシとコストを最小化します。
	ThinkingMinimal ThinkingLevel = genai.ThinkingLevelMinimal
	// ThinkingLow は軽い思考を行います。
	ThinkingLow ThinkingLevel = genai.ThinkingLevelLow
	// ThinkingMedium は中程度の思考を行います。
	ThinkingMedium ThinkingLevel = genai.ThinkingLevelMedium
	// ThinkingHigh は最も深い思考を行います。品質は上がりますが遅く高価です。
	ThinkingHigh ThinkingLevel = genai.ThinkingLevelHigh
)

// SafetyThreshold は、安全フィルタのブロック閾値です（genai.HarmBlockThreshold の別名）。
type SafetyThreshold = genai.HarmBlockThreshold

// 安全フィルタのブロック閾値です。
const (
	// SafetyBlockNone は、安全フィルタによるブロックを行いません。
	SafetyBlockNone SafetyThreshold = genai.HarmBlockThresholdBlockNone
	// SafetyBlockLowAndAbove は、低リスク以上をブロックする最も厳しい設定です。
	SafetyBlockLowAndAbove SafetyThreshold = genai.HarmBlockThresholdBlockLowAndAbove
	// SafetyBlockMediumAndAbove は、中リスク以上をブロックします。
	SafetyBlockMediumAndAbove SafetyThreshold = genai.HarmBlockThresholdBlockMediumAndAbove
	// SafetyBlockOnlyHigh は、高リスクのみをブロックします。
	SafetyBlockOnlyHigh SafetyThreshold = genai.HarmBlockThresholdBlockOnlyHigh
	// SafetyOff は、安全フィルタ自体を無効にします。BlockNone との違いはバックエンドの
	// 対応状況によるため、意図して使い分ける場合以外は SafetyBlockNone を選んでください。
	SafetyOff SafetyThreshold = genai.HarmBlockThresholdOff
)

// Schema は、構造化出力（GenerateOptions.ResponseSchema）のスキーマです（genai.Schema の別名）。
//
// map[string]any で書く ResponseJSONSchema と違い、フィールド名がコンパイル時に
// 検査されます。JSON Schema の $ref のようにこの型で表現しきれない場合にだけ
// ResponseJSONSchema を選んでください。
//
//	opts := gemini.GenerateOptions{
//		ResponseMIMEType: "application/json",
//		ResponseSchema: &gemini.Schema{
//			Type: gemini.TypeObject,
//			Properties: map[string]*gemini.Schema{
//				"title": {Type: gemini.TypeString},
//			},
//			Required: []string{"title"},
//		},
//	}
type Schema = genai.Schema

// SchemaType は、スキーマのデータ型です（genai.Type の別名）。
type SchemaType = genai.Type

// スキーマのデータ型です。
const (
	// TypeString は文字列です。
	TypeString SchemaType = genai.TypeString
	// TypeNumber は浮動小数点数です。
	TypeNumber SchemaType = genai.TypeNumber
	// TypeInteger は整数です。
	TypeInteger SchemaType = genai.TypeInteger
	// TypeBoolean は真偽値です。
	TypeBoolean SchemaType = genai.TypeBoolean
	// TypeArray は配列です。要素の型は Items で指定します。
	TypeArray SchemaType = genai.TypeArray
	// TypeObject はオブジェクトです。フィールドは Properties で指定します。
	TypeObject SchemaType = genai.TypeObject
)

// NewSafetySettings は、標準的な4つのハームカテゴリ（暴力・ヘイト・性的表現・危険行為）
// すべてに同一の閾値を適用した SafetySetting のスライスを返します。
// 閾値をバックエンドや用途に応じてどう選ぶかは呼び出し側の判断に委ねます。
func NewSafetySettings(threshold SafetyThreshold) []*genai.SafetySetting {
	return []*genai.SafetySetting{
		{Category: genai.HarmCategoryHarassment, Threshold: threshold},
		{Category: genai.HarmCategoryHateSpeech, Threshold: threshold},
		{Category: genai.HarmCategorySexuallyExplicit, Threshold: threshold},
		{Category: genai.HarmCategoryDangerousContent, Threshold: threshold},
	}
}

// seedToPtrInt32 は *int64 を SDK 用の *int32 に変換します。
func seedToPtrInt32(s *int64) (*int32, error) {
	if s == nil {
		return nil, nil
	}

	if *s > math.MaxInt32 || *s < math.MinInt32 {
		return nil, fmt.Errorf("%w (入力値: %d)", ErrInvalidSeed, *s)
	}

	return new(int32(*s)), nil
}
