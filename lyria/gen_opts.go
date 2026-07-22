package lyria

import (
	"github.com/shouni/go-gemini-client/gemini"
	"google.golang.org/genai"
)

// buildJSONGenerateOptions は Gemini による JSON 形式の構造化データ生成に最適化されたオプションを返します。
// schema を指定すると構造化出力（constrained decoding）が有効になり、出力が文法レベルで制約されます。
func buildJSONGenerateOptions(seed *int64, schema *genai.Schema) gemini.GenerateOptions {
	opts := buildBaseOptions(seed, "application/json")
	opts.ResponseSchema = schema
	return opts
}

// buildAudioGenerateOptions は Lyria による音声生成に最適化されたオプションを返します。
// Lyria モデルはレスポンス MIME type の指定なしで音声を返すため、指定しません。
func buildAudioGenerateOptions(seed *int64) gemini.GenerateOptions {
	return buildBaseOptions(seed, "")
}

// buildBaseOptions はパッケージ共通の安全設定やシード値を適用したベースオプションを構築します。
// NOTE: 生成結果の再現性を優先するため、対応カテゴリのブロック閾値は BlockNone に統一しています。
// 入力・出力の制御は呼び出し側または後段処理で行う前提です。
func buildBaseOptions(seed *int64, mimeType string) gemini.GenerateOptions {
	opts := gemini.GenerateOptions{
		SafetySettings: gemini.NewSafetySettings(genai.HarmBlockThresholdBlockNone),
	}
	if seed != nil {
		opts.Seed = seed
	}
	if mimeType != "" {
		opts.ResponseMIMEType = mimeType
	}
	return opts
}
