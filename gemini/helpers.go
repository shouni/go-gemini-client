package gemini

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"

	"google.golang.org/genai"
)

// APIResponseError は、コンテンツのブロックや空のレスポンスなど、
// APIとの通信成功後に発生した論理的なエラーを示します。
type APIResponseError struct {
	msg string
}

func (e *APIResponseError) Error() string { return e.msg }

// shouldRetry は、発生したエラーがリトライによって解決可能かどうかを判定します。
func shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// 安全フィルターによるブロック等の論理エラーはリトライしても解決しないため即座に終了します。
	if _, ok := errors.AsType[*APIResponseError](err); ok {
		return false
	}

	// コンテキストのキャンセルやタイムアウト（呼び出し側管理）はリトライ対象外です。
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// genai SDK は REST で通信し、API エラーを HTTP ステータスコード付きの
	// genai.APIError（値型）として返すため、ステータスコードで判定します。
	if apiErr, ok := errors.AsType[genai.APIError](err); ok {
		switch apiErr.Code {
		case http.StatusTooManyRequests, // レート制限
			http.StatusInternalServerError, // サーバー内部エラー
			http.StatusServiceUnavailable,  // 一時的なサービス停止
			http.StatusGatewayTimeout:      // サーバー側でのタイムアウト
			return true
		default:
			return false
		}
	}

	// gRPCエラー以外（ネットワーク接続エラー、EOFなど）は一時的な障害の可能性があるためリトライを許可します。
	if errors.Is(err, io.EOF) {
		return true
	}
	if netErr, ok := errors.AsType[net.Error](err); ok {
		return netErr.Timeout()
	}

	return false
}

// extractTextFromResponse はレスポンスからテキストを抽出し、異常な終了理由がないか確認します。
func extractTextFromResponse(resp *genai.GenerateContentResponse) (string, error) {
	if resp == nil || len(resp.Candidates) == 0 {
		return "", &APIResponseError{msg: "Gemini APIから空のレスポンスが返されました"}
	}

	candidate := resp.Candidates[0]

	// FinishReason が正常（指定なし または STOP）以外の場合は、ブロックされたとみなします。
	if candidate.FinishReason != genai.FinishReasonUnspecified && candidate.FinishReason != genai.FinishReasonStop {
		return "", &APIResponseError{msg: fmt.Sprintf("生成がブロックされました（理由: %v）", candidate.FinishReason)}
	}

	// コンテンツが存在しない場合。
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", nil
	}

	// 最初に見つかったテキストパーツを返します。
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			return part.Text, nil
		}
	}

	return "", nil
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
