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

	end := strings.LastIndex(input, "}")
	if end != -1 && end >= start {
		candidate := input[start : end+1]
		if json.Valid([]byte(candidate)) {
			return candidate
		}
	}

	// LLM が '}' の代わりに ')' などで閉じてしまうケースを補正する。
	trimmed := strings.TrimRight(input[start:], " \t\n\r),;")
	repaired := trimmed + "}"
	if json.Valid([]byte(repaired)) {
		return repaired
	}

	return input
}
