package gemini

import (
	"encoding/json"
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

// TestCleanJSONResponseRepairsBrokenStrings は、文字列リテラルの中の崩れを補修する
// ことを検証します。構造化出力を指定していても起こり、応答を返しきったあとの崩れ
// なので API の再試行では直りません。台本の抜粋・歌詞・台詞のように複数行の本文を
// JSON に載せる用途で出ます。
func TestCleanJSONResponseRepairsBrokenStrings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "raw newline inside a string value",
			input: "{\"excerpt\":\"1行目\n2行目\"}",
			want:  `{"excerpt":"1行目\n2行目"}`,
		},
		{
			name:  "raw tab inside a string value",
			input: "{\"code\":\"if x:\n\tpass\"}",
			want:  `{"code":"if x:\n\tpass"}`,
		},
		{
			name:  "lone backslash inside a string value",
			input: `{"pattern":"\d+件"}`,
			want:  `{"pattern":"\\d+件"}`,
		},
		{
			name:  "escaped quote survives the backslash repair",
			input: `{"quote":"彼は\"はい\"と言った\d"}`,
			want:  `{"quote":"彼は\"はい\"と言った\\d"}`,
		},
		{
			name:  "fenced block with a raw newline inside a string",
			input: "```json\n{\"lyrics\":\"A\nB\"}\n```",
			want:  `{"lyrics":"A\nB"}`,
		},
		{
			name:  "raw newline plus trailing prose",
			input: "{\"a\":\"x\ny\"} 以上です。",
			want:  `{"a":"x\ny"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := CleanJSONResponse(tt.input)
			assert.Equal(t, tt.want, got)
			assert.True(t, json.Valid([]byte(got)), "補修後は妥当な JSON である必要があります")
		})
	}
}

// TestCleanJSONResponseNeverWorsens は、直せない入力を悪化させないことを検証します。
// 呼び出し側のエラーメッセージが元の壊れ方を指したままになるようにするためです。
func TestCleanJSONResponseNeverWorsens(t *testing.T) {
	t.Parallel()

	unrepairable := []string{
		`{"a": }`,
		`{"unterminated": "文字列が閉じていません`,
		`ここには JSON がありません`,
		"",
	}

	for _, in := range unrepairable {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, in, CleanJSONResponse(in))
		})
	}
}

// TestCleanJSONResponseLeavesValidInputAlone は、解釈できる入力を 1 バイトも
// 変えないことを検証します。整形に使われた改行やインデントも保ちます。
func TestCleanJSONResponseLeavesValidInputAlone(t *testing.T) {
	t.Parallel()

	valid := []string{
		`{"a":1}`,
		"{\n  \"a\": 1,\n  \"b\": [2, 3]\n}",
		`[{"a":1},{"b":2}]`,
		`{"path":"C:\\tmp\\a.txt"}`,
		`{"text":"改行は\nこう"}`,
	}

	for _, in := range valid {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, in, CleanJSONResponse(in))
		})
	}
}
