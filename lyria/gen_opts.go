package lyria

import (
	"github.com/shouni/go-gemini-client/gemini"
	"google.golang.org/genai"
)

// buildJSONGenerateOptions は Gemini による JSON 形式の構造化データ生成に最適化されたオプションを返します。
func buildJSONGenerateOptions(seed *int64) gemini.GenerateOptions {
	return buildBaseOptions(seed, "application/json")
}

// buildAudioGenerateOptions は Lyria による音声生成に最適化されたオプションを返します。
func buildAudioGenerateOptions(seed *int64, mimeType string) gemini.GenerateOptions {
	return buildBaseOptions(seed, mimeType)
}

// buildBaseOptions は AP Comp 全体で共通の安全設定やシード値を適用したベースオプションを構築します。
func buildBaseOptions(seed *int64, mimeType string) gemini.GenerateOptions {
	return gemini.GenerateOptions{
		ResponseMIMEType: mimeType,
		Seed:             seed,
		SafetySettings:   buildSafetySettings(),
	}
}

// buildSafetySettings は AP Comp 共通の安全性設定を返します。
// NOTE: 生成結果の再現性を優先するため、対応カテゴリのブロック閾値は BlockNone に統一しています。
// 入力・出力の制御は呼び出し側または後段処理で行う前提です。
func buildSafetySettings() []*genai.SafetySetting {
	return []*genai.SafetySetting{
		{Category: genai.HarmCategoryHarassment, Threshold: genai.HarmBlockThresholdBlockNone},
		{Category: genai.HarmCategoryHateSpeech, Threshold: genai.HarmBlockThresholdBlockNone},
		{Category: genai.HarmCategorySexuallyExplicit, Threshold: genai.HarmBlockThresholdBlockNone},
		{Category: genai.HarmCategoryDangerousContent, Threshold: genai.HarmBlockThresholdBlockNone},
	}
}
