package gemini

import (
	"encoding/json"
	"strings"
)

// CleanJSONResponse は、GenerateOptions.ResponseSchema による構造化出力を使っても
// なお LLM が出力しがちな Markdown の装飾や末尾ノイズを除去・補正します。
// responseMimeType に "application/json" を指定しても、モデルが完結した JSON の後に
// 余分な閉じ括弧や説明テキストを継ぎ足すケースが実際に確認されているため、
// json.Unmarshal の前段でこの関数を通すことを推奨します。
func CleanJSONResponse(input string) string {
	start, closer := firstJSONStart(input)
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
	trimmed := strings.TrimRight(input[start:], " \t\n\r)]},;")
	repaired := trimmed + closer
	if json.Valid([]byte(repaired)) {
		return repaired
	}

	return input
}

// firstJSONStart は最初に現れる JSON 値の開始位置と、対応する閉じ括弧を返します。
// トップレベルが配列のスキーマ（例: 章立てのリスト）にも対応するため、
// '{' と '[' の早い方を採用します。
func firstJSONStart(input string) (start int, closer string) {
	obj := strings.Index(input, "{")
	arr := strings.Index(input, "[")

	switch {
	case obj == -1 && arr == -1:
		return -1, ""
	case arr == -1 || (obj != -1 && obj < arr):
		return obj, "}"
	default:
		return arr, "]"
	}
}
