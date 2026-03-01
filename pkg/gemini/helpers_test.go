package gemini

import (
	"context"
	"io"
	"math"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- buildRetryConfig のテスト ---
func TestBuildRetryConfig(t *testing.T) {
	t.Run("デフォルト値が適用されること", func(t *testing.T) {
		cfg := Config{} // 全て nil の状態
		got := buildRetryConfig(cfg)
		if got.MaxRetries != DefaultMaxRetries {
			t.Errorf("MaxRetries = %v, want %v", got.MaxRetries, DefaultMaxRetries)
		}
	})

	t.Run("設定値で上書きされること", func(t *testing.T) {
		cfg := Config{
			MaxRetries:   5,
			InitialDelay: 10 * time.Second,
			MaxDelay:     60 * time.Second,
		}
		got := buildRetryConfig(cfg)
		if got.MaxRetries != 5 || got.InitialInterval != 10*time.Second || got.MaxInterval != 60*time.Second {
			t.Errorf("設定が正しく適用されていません: %+v", got)
		}
	})
}

// --- shouldRetry のテスト ---
func TestShouldRetry(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nilはリトライしない", nil, false},
		{"APIResponseErrorはリトライしない", &APIResponseError{msg: "blocked"}, false},
		{"コンテキストキャンセルはリトライしない", context.Canceled, false},
		{"gRPC Unavailableはリトライする", status.Error(codes.Unavailable, "service down"), true},
		{"gRPC Internalはリトライする", status.Error(codes.Internal, "internal error"), true},
		{"gRPC InvalidArgumentはリトライしない", status.Error(codes.InvalidArgument, "bad request"), false},
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

// --- seedToPtrInt32 のテスト ---
func TestSeedToPtrInt32(t *testing.T) {
	validSeed := int64(12345)
	overSeed := int64(math.MaxInt32 + 1)

	tests := []struct {
		name  string
		input *int64
		want  *int32
	}{
		{"nilならnil", nil, nil},
		{"正常な範囲", &validSeed, int32Ptr(12345)},
		{"int32範囲外ならnil", &overSeed, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := seedToPtrInt32(tt.input)
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("seedToPtrInt32() の結果（nilかどうか）が一致しません: got %v, want %v", got, tt.want)
			}
			if got != nil && *got != *tt.want {
				t.Errorf("seedToPtrInt32() = %v, want %v", *got, *tt.want)
			}
		})
	}
}

// --- ヘルパー関数 (テスト用) ---
func float32Ptr(f float32) *float32 { return &f }
func int32Ptr(i int32) *int32       { return &i }
