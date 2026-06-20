package lyria

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanJSONResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain JSON",
			input: `{"title":"test"}`,
			want:  `{"title":"test"}`,
		},
		{
			name:  "markdown code block",
			input: "```json\n{\"title\":\"test\"}\n```",
			want:  `{"title":"test"}`,
		},
		{
			name:  "leading and trailing text",
			input: `Here is the JSON: {"title":"test"} done.`,
			want:  `{"title":"test"}`,
		},
		{
			name:  "nested JSON",
			input: `{"a":{"b":"c"}}`,
			want:  `{"a":{"b":"c"}}`,
		},
		{
			name:  "invalid JSON returns original",
			input: `{"unclosed"`,
			want:  `{"unclosed"`,
		},
		{
			name:  "no braces returns original",
			input: `no json here`,
			want:  `no json here`,
		},
		{
			name:  "invalid JSON extracted from text returns original",
			input: `prefix {broken json} suffix`,
			want:  `prefix {broken json} suffix`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, cleanJSONResponse(tt.input))
		})
	}
}
