package gemini

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
		{
			name:  "missing closing brace replaced by paren",
			input: "{\"title\":\"test\",\"narrative\":\"hello\")",
			want:  `{"title":"test","narrative":"hello"}`,
		},
		{
			name:  "missing closing brace with trailing whitespace",
			input: "{\"title\":\"test\")\n",
			want:  `{"title":"test"}`,
		},
		{
			// 本番障害の実パターン: 完結した JSON の後に余分な '}' と本文の断片が続く
			name:  "trailing extra brace and prose after valid JSON",
			input: "{\n  \"title\": \"調和の翼\",\n  \"narrative\": \"王道アニソン。\"\n}\n}\nアニソンファンタジー。\"\n})",
			want:  "{\n  \"title\": \"調和の翼\",\n  \"narrative\": \"王道アニソン。\"\n}",
		},
		{
			name:  "trailing prose without extra brace",
			input: `{"title":"test"} これは補足の説明です。`,
			want:  `{"title":"test"}`,
		},
		{
			name:  "braces inside string values",
			input: `{"lyrics":"[Verse]\n光 {影} 空"} garbage }`,
			want:  `{"lyrics":"[Verse]\n光 {影} 空"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, CleanJSONResponse(tt.input))
		})
	}
}
