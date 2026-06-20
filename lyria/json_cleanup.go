package lyria

import (
	"encoding/json"
	"strings"
)

// cleanJSONResponse は LLM が出力しがちな Markdown の装飾を除去します。
func cleanJSONResponse(input string) string {
	start := strings.Index(input, "{")
	end := strings.LastIndex(input, "}")
	if start == -1 || end == -1 || start > end {
		return input
	}
	candidate := input[start : end+1]
	if !json.Valid([]byte(candidate)) {
		return input
	}
	return candidate
}
