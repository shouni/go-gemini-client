package gemini

import (
	"encoding/json"
	"strings"
	"testing"
)

// FuzzCleanJSONResponse は、LLM 出力のノイズ除去が panic せず、
// かつ「入力を変えた結果として不正な JSON を作り出さない」ことを検証します。
//
// この関数は json.Unmarshal の前段で必ず通るため、ここが壊れると
// 構造化出力の経路全体が壊れます。
func FuzzCleanJSONResponse(f *testing.F) {
	seeds := []string{
		`{"title":"ok"}`,
		"```json\n{\"title\":\"ok\"}\n```",
		`{"title":"ok"} 余計な説明文`,
		`{"title":"ok"}}`,
		`{"title":"ok"`,
		`{"title":"ok")`,
		`{"title":"波括弧 } を含む文字列"}`,
		`[{"a":1},{"b":2}]`,
		`前置き [1,2,3] 後置き`,
		`[1,2,3]]`,
		`[{"a":1}`,
		`{"nested":{"deep":[1,{"x":"}"}]}}`,
		"",
		"no json here",
		"{",
		"[",
		"}{",
		`{"a":"\"escaped\""}`,
		strings.Repeat("{", 128),
		// 文字列リテラルの中の崩れ（生の制御文字・裸のバックスラッシュ）。
		"{\"a\":\"1行目\n2行目\"}",
		"{\"a\":\"tab\there\"}",
		`{"a":"\d+"}`,
		`{"a":"\\d"}`,
		`{"a":"彼は\"はい\"と言った\d"}`,
		"```json\n{\"a\":\"A\nB\"}\n```",
		"{\"a\":\"x\ny\"} 以上です。",
		"{\"a\":\"\x00\"}",
		`{"a":"\u00"}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		got := CleanJSONResponse(raw)

		// 出力が入力と同じなら「補正できなかった」ことを意味するので不変条件はない。
		if got == raw {
			return
		}

		// 入力を書き換えた以上、その結果は必ず妥当な JSON でなければならない。
		// そうでないと呼び出し側の json.Unmarshal が、元の入力より悪い状態で失敗する。
		if !json.Valid([]byte(got)) {
			t.Fatalf("入力を書き換えたのに不正な JSON になりました\n入力: %q\n出力: %q", raw, got)
		}
	})
}
