package gemini

import (
	"errors"
	"math"
	"testing"
)

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
			got, err := seedToPtrInt32(tt.input)
			if tt.want == nil && tt.input != nil {
				if err == nil {
					t.Fatal("範囲外の Seed でエラーが返されませんでした")
				}
				if !errors.Is(err, ErrInvalidSeed) {
					t.Fatalf("seedToPtrInt32() error = %v, want %v", err, ErrInvalidSeed)
				}
				return
			}
			if err != nil {
				t.Fatalf("seedToPtrInt32() unexpected error = %v", err)
			}
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("seedToPtrInt32() の結果（nilかどうか）が一致しません: got %v, want %v", got, tt.want)
			}
			if got != nil && *got != *tt.want {
				t.Errorf("seedToPtrInt32() = %v, want %v", *got, *tt.want)
			}
		})
	}
}

func int32Ptr(i int32) *int32 { return &i }
