package gemini

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"google.golang.org/genai"
)

// --- shouldRetry のテスト ---
func TestShouldRetry(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nilはリトライしない", nil, false},
		{"APIResponseErrorはリトライしない", newBlockedError(genai.FinishReasonSafety), false},
		{"コンテキストキャンセルはリトライしない", context.Canceled, false},
		{"429 レート制限はリトライする", genai.APIError{Code: http.StatusTooManyRequests, Status: "RESOURCE_EXHAUSTED"}, true},
		{"500 サーバーエラーはリトライする", genai.APIError{Code: http.StatusInternalServerError, Status: "INTERNAL"}, true},
		{"503 一時停止はリトライする", genai.APIError{Code: http.StatusServiceUnavailable, Status: "UNAVAILABLE"}, true},
		{"504 タイムアウトはリトライする", genai.APIError{Code: http.StatusGatewayTimeout, Status: "DEADLINE_EXCEEDED"}, true},
		{"400 リクエスト不正はリトライしない", genai.APIError{Code: http.StatusBadRequest, Status: "INVALID_ARGUMENT"}, false},
		{"404 未検出はリトライしない", genai.APIError{Code: http.StatusNotFound, Status: "NOT_FOUND"}, false},
		{"ラップされた429もリトライする", fmt.Errorf("wrapped: %w", genai.APIError{Code: http.StatusTooManyRequests}), true},
		{"EOFはリトライする", io.EOF, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetry(tt.err); got != tt.want {
				t.Errorf("shouldRetry(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
