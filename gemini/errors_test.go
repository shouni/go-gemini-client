package gemini

import (
	"errors"
	"testing"

	"google.golang.org/genai"
)

func TestAPIResponseError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *APIResponseError
		want string
	}{
		{
			name: "Message を優先する",
			err:  &APIResponseError{Reason: ErrBlocked, Message: "明示メッセージ"},
			want: "明示メッセージ",
		},
		{
			name: "Message が空なら FinishReason から組み立てる",
			err:  &APIResponseError{Reason: ErrBlocked, FinishReason: genai.FinishReasonSafety},
			want: "生成がブロックされました（理由: SAFETY）",
		},
		{
			name: "Message も FinishReason も無ければ Reason を返す",
			err:  &APIResponseError{Reason: ErrEmptyResponse},
			want: ErrEmptyResponse.Error(),
		},
		{
			name: "何も無ければ既定メッセージ",
			err:  &APIResponseError{},
			want: "gemini: API response error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPIResponseError_Unwrap(t *testing.T) {
	t.Run("Reason で分類できること", func(t *testing.T) {
		err := newBlockedError(genai.FinishReasonRecitation)

		if !errors.Is(err, ErrBlocked) {
			t.Error("ErrBlocked に一致しません")
		}
		if errors.Is(err, ErrEmptyResponse) {
			t.Error("ErrEmptyResponse に誤って一致しています")
		}
	})

	t.Run("Reason が nil でも panic しないこと", func(t *testing.T) {
		err := &APIResponseError{Message: "x"}
		if errors.Unwrap(err) != nil {
			t.Error("nil Reason は nil を返すべきです")
		}
	})
}
