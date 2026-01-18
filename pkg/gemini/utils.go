package gemini

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"

	"google.golang.org/genai"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	var apiErr *APIResponseError
	if errors.As(err, &apiErr) {
		return false
	}

	// コンテキストのキャンセルやタイムアウト（呼び出し側管理）はリトライ対象外です。
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// gRPC ステータスコードに基づいた判定。
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.DeadlineExceeded, // サーバー側でのタイムアウト
			codes.Unavailable,       // 一時的なサービス停止
			codes.ResourceExhausted, // レート制限
			codes.Internal:          // サーバー内部エラー
			return true
		default:
			return false
		}
	}

	// gRPCエラー以外（ネットワーク接続エラー、EOFなど）は一時的な障害の可能性があるためリトライを許可します。
	if errors.Is(err, io.EOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Temporary() || netErr.Timeout()
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
// 範囲外の場合は slog で警告を記録し、nil を返して処理を続行します。
func seedToPtrInt32(s *int64) *int32 {
	if s == nil {
		return nil
	}

	if *s > math.MaxInt32 || *s < math.MinInt32 {
		slog.Warn("シード値が int32 の許容範囲を超えています。シードを指定せずに処理を続行します。",
			"入力値", *s,
			"最大値", math.MaxInt32,
			"最小値", math.MinInt32,
		)
		return nil
	}

	v := int32(*s)
	return &v
}
