package gemini

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/shouni/netarmor/retry"
	"google.golang.org/genai"
)

// runWithRetry は共通のリトライ設定を適用して操作を実行します。
// retry.RunValue を使うことで、呼び出し側がクロージャで結果を受け渡す必要がなくなります。
func runWithRetry[T any](ctx context.Context, opts []retry.Option, name string, op func() (T, error)) (T, error) {
	all := make([]retry.Option, 0, len(opts)+2)
	all = append(all, opts...)
	all = append(all, retry.WithName(name), retry.WithShouldRetry(shouldRetry))
	return retry.RunValue(ctx, op, all...)
}

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

	// APIError に分類されないエラー（ネットワーク接続エラー、EOFなど）は一時的な障害の可能性があるためリトライを許可します。
	if errors.Is(err, io.EOF) {
		return true
	}
	if netErr, ok := errors.AsType[net.Error](err); ok {
		return netErr.Timeout()
	}

	return false
}
