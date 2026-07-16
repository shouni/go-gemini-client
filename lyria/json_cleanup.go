package lyria

import (
	"encoding/json"
	"strings"
)

// cleanJSONResponse は LLM が出力しがちな Markdown の装飾や末尾ノイズを除去・補正します。
func cleanJSONResponse(input string) string {
	start := strings.Index(input, "{")
	if start == -1 {
		return input
	}

	// 最初の完結した JSON 値だけを取り出す。json.Decoder は文字列リテラル内の
	// 括弧も正しく扱いながらバランスの取れた位置で停止するため、値の後ろに
	// 続く余分な '}' や説明テキストなどのノイズを無視できる。
	var obj json.RawMessage
	if err := json.NewDecoder(strings.NewReader(input[start:])).Decode(&obj); err == nil {
		return string(obj)
	}

	// LLM が '}' の代わりに ')' などで閉じてしまうケースを補正する。
	trimmed := strings.TrimRight(input[start:], " \t\n\r),;")
	repaired := trimmed + "}"
	if json.Valid([]byte(repaired)) {
		return repaired
	}

	return input
}
