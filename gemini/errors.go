package gemini

import (
	"errors"
	"fmt"

	"google.golang.org/genai"
)

// 入力検証エラー。
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
	// ErrEmptyOperationName は、オペレーション名が空の場合に返されます。
	ErrEmptyOperationName = errors.New("オペレーション名を空にすることはできません")
	// ErrInvalidVideoInput は、動画生成の入力の組み合わせが API の受け付けない
	// ものだった場合に返されます（Image と Video の併用など）。
	ErrInvalidVideoInput = errors.New("動画生成の入力の組み合わせが不正です")
)

// ErrVideoGenerationFailed は、動画生成のオペレーションが失敗として完了したことを
// 示します。オペレーションの取得自体は成功しているため、通信エラーとは区別されます。
// VideoOperation.Failure に載る形で返されます。
var ErrVideoGenerationFailed = errors.New("gemini: video generation failed")

// API との通信は成功したが、レスポンス内容が利用できない場合のセンチネルエラー。
// いずれもリトライでは解決しないため shouldRetry は false を返します。
var (
	// ErrBlocked は、安全フィルタ等により生成がブロックされたことを示します。
	// プロンプトを変えない限り再試行しても同じ結果になります。
	// 詳細な理由は errors.AsType[*APIResponseError] で FinishReason を参照してください。
	ErrBlocked = errors.New("gemini: generation blocked")

	// ErrEmptyResponse は、候補が 1 件も含まれないレスポンスが返されたことを示します。
	ErrEmptyResponse = errors.New("gemini: empty response")
)

// APIResponseError は、コンテンツのブロックや空のレスポンスなど、
// APIとの通信成功後に発生した論理的なエラーを示します。
//
// errors.Is により ErrBlocked / ErrEmptyResponse のいずれかと比較できます。
//
//	if errors.Is(err, gemini.ErrBlocked) {
//	    // プロンプトを見直す
//	}
//	if apiErr, ok := errors.AsType[*gemini.APIResponseError](err); ok {
//	    slog.Warn("blocked", "reason", apiErr.FinishReason)
//	}
type APIResponseError struct {
	// Reason は分類用のセンチネル（ErrBlocked または ErrEmptyResponse）です。
	Reason error
	// FinishReason は、ブロック時にモデルが返した終了理由です。
	// 空レスポンスなど終了理由が無い場合はゼロ値（空文字列）になります。
	FinishReason genai.FinishReason
	// Message は人間向けの説明です。
	Message string
}

func (e *APIResponseError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if !isUnsetFinishReason(e.FinishReason) {
		return fmt.Sprintf("生成がブロックされました（理由: %v）", e.FinishReason)
	}
	if e.Reason != nil {
		return e.Reason.Error()
	}
	return "gemini: API response error"
}

// Unwrap は分類用センチネルを返し、errors.Is による判定を可能にします。
func (e *APIResponseError) Unwrap() error { return e.Reason }

// isUnsetFinishReason は、終了理由が「設定されていない」かを判定します。
//
// genai.FinishReason は文字列型で、Go のゼロ値は "" ですが、SDK 定数の
// FinishReasonUnspecified は "FINISH_REASON_UNSPECIFIED" という別の値です。
// ストリーミングの中間チャンクなど終了理由を含まないレスポンスではゼロ値になるため、
// 両方を「未設定」として扱わないとブロック扱いに誤判定します。
func isUnsetFinishReason(r genai.FinishReason) bool {
	return r == "" || r == genai.FinishReasonUnspecified
}

// isBlockedFinishReason は、終了理由が異常終了（ブロック等）を示すかを判定します。
func isBlockedFinishReason(r genai.FinishReason) bool {
	return !isUnsetFinishReason(r) && r != genai.FinishReasonStop
}

// newBlockedError は FinishReason 付きのブロックエラーを生成します。
func newBlockedError(reason genai.FinishReason) *APIResponseError {
	return &APIResponseError{
		Reason:       ErrBlocked,
		FinishReason: reason,
		Message:      fmt.Sprintf("生成がブロックされました（理由: %v）", reason),
	}
}

// newEmptyResponseError は空レスポンスエラーを生成します。
func newEmptyResponseError() *APIResponseError {
	return &APIResponseError{
		Reason:  ErrEmptyResponse,
		Message: "Gemini APIから空のレスポンスが返されました",
	}
}
