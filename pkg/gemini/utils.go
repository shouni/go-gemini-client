package gemini

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/genai"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// APIResponseError は生成ブロックや空レスポンスなど、通信成功後の論理的なエラーを示すのだ。
type APIResponseError struct {
	msg string
}

func (e *APIResponseError) Error() string { return e.msg }

// shouldRetry は発生したエラーがリトライで解決可能かどうかを判定するのだ。
func shouldRetry(err error) bool {
	// 規約違反（ブロック）などはリトライしても無駄なので即座に諦めるのだ
	var apiErr *APIResponseError
	if errors.As(err, &apiErr) {
		return false
	}

	// キャンセルやタイムアウト（上位管理）もリトライ対象外なのだ
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// gRPC のステータスコードを元に、一時的な障害のみリトライを許可するのだ
	st, ok := status.FromError(err)
	if !ok {
		// gRPCエラーでない場合は、基本的な接続エラーの可能性があるため安全側に倒してリトライさせない
		return false
	}

	switch st.Code() {
	case codes.DeadlineExceeded, // 処理がタイムアウトした場合
		codes.Unavailable,       // サーバーが一時的にダウンしている場合
		codes.ResourceExhausted, // レート制限（429）に達した場合
		codes.Internal:          // サーバー内部エラー（500）
		return true
	default:
		return false
	}
}

// extractTextFromResponse はレスポンスからテキストを安全に抽出し、異常な終了理由がないか確認するのだ。
func extractTextFromResponse(resp *genai.GenerateContentResponse) (string, error) {
	if resp == nil || len(resp.Candidates) == 0 {
		return "", &APIResponseError{msg: "Gemini APIから空のレスポンスが返されました"}
	}

	candidate := resp.Candidates[0]

	// FinishReason が正常（指定なし or 停止）以外なら、安全フィルター等によるブロックとみなすのだ
	if candidate.FinishReason != genai.FinishReasonUnspecified && candidate.FinishReason != genai.FinishReasonStop {
		return "", &APIResponseError{msg: fmt.Sprintf("生成がブロックされました。理由: %v", candidate.FinishReason)}
	}

	// 画像生成の場合、Content自体が空でもエラーにせず続行させるのだ（画像データは別途取得可能なため）
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", nil
	}

	// Partsの中から最初に見つかったテキストを返すのだ
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			return part.Text, nil
		}
	}

	// テキスト部分が含まれていない場合も正常として扱う（画像のみの応答などのケース）
	return "", nil
}
