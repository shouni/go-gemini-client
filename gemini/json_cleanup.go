package gemini

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CleanJSONResponse は、AI が返した JSON をそのままでは解釈できないときに補修します。
//
// GenerateOptions.ResponseSchema / ResponseJSONSchema で構造化出力を指定しても、
// モデルは次の形で崩すことがあります。**どれも応答を返しきったあとの話なので、
// API の再試行では直りません。**
//
//   - Markdown のフェンス（```json … ```）で包む
//   - 完結した JSON の後ろに説明文や余分な閉じ括弧を継ぎ足す
//   - '}' の代わりに ')' などで閉じる
//   - 文字列の中でバックスラッシュをエスケープし忘れる（ソースやパスを引用したとき）
//   - 文字列の中に改行やタブを生のまま入れる（複数行の本文を引用したとき）
//
// 最後の 2 つは、台本の抜粋・歌詞・台詞のように**複数行の本文を JSON に載せる用途で
// 特に起きます。** ここで直さないと、数分かけた生成が「解釈できません」の一行だけを
// 残して丸ごと失われます。
//
// **既に解釈できる入力は 1 バイトも変えません。** 補修しても妥当な JSON にならなければ
// 入力をそのまま返します。呼び出し側のエラーメッセージが元の壊れ方を指したままになるよう、
// 悪化させないことを優先します。
//
// go-review-kit の review.SanitizeJSON が同じ問題を解いています。あちらは go.mod を
// go-git 1 本に絞る方針なのでこのモジュールへは寄せず、意図的に別実装のままにしています
// （切り詰められた応答からの救出は、データを落とすうえ呼び出し側へ通知する口が要るため、
// あちらにしかありません）。
func CleanJSONResponse(input string) string {
	if json.Valid([]byte(input)) {
		return input
	}

	if cleaned, ok := extractJSONValue(input); ok {
		return cleaned
	}

	// 文字列の中の壊れたエスケープは、括弧の対応では直りません。
	// バックスラッシュと生の制御文字を補ってからもう一度、値の切り出しを試みます。
	//
	// 順序に意味があります。制御文字の補修は `\n` のようにバックスラッシュを**足す**ので、
	// 後から裸のバックスラッシュを探すと、足したばかりのエスケープを二重にしてしまいます。
	escaped := escapeControlChars(escapeLoneBackslashes(input))
	if escaped != input {
		if json.Valid([]byte(escaped)) {
			return escaped
		}
		if cleaned, ok := extractJSONValue(escaped); ok {
			return cleaned
		}
	}

	return input
}

// extractJSONValue は、最初に現れる完結した JSON 値を取り出します。
// 取り出せなければ ok が false です。
func extractJSONValue(input string) (string, bool) {
	start, closer := firstJSONStart(input)
	if start == -1 {
		return "", false
	}

	// json.Decoder は文字列リテラルの中の括弧を正しく読み飛ばしたうえで、値が閉じた
	// 位置で止まります。前後に付いたフェンスや説明文を、括弧を数えずに落とせるのが要点です。
	var value json.RawMessage
	if err := json.NewDecoder(strings.NewReader(input[start:])).Decode(&value); err == nil {
		return string(value), true
	}

	// LLM が '}' の代わりに ')' などで閉じてしまうケースを補正します。
	trimmed := strings.TrimRight(input[start:], " \t\n\r)]},;")
	if repaired := trimmed + closer; json.Valid([]byte(repaired)) {
		return repaired, true
	}

	return "", false
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

// escapeLoneBackslashes は、文字列リテラルの中でエスケープされていない
// バックスラッシュを二重にします。
//
// JSON でバックスラッシュの後に来られるのは "\/bfnrtu だけなので、それ以外が続く `\X` は
// **モデルがエスケープし忘れた literal なバックスラッシュ**と一意に決まります
// （正規表現の `\d` やパスを本文へ引用したときに出ます）。`\X` → `\\X` は情報を落とさず、
// 解釈が二通りになることもありません。
//
// 文字列の外は触りません。JSON の構文としてバックスラッシュが現れるのは文字列の中だけです。
func escapeLoneBackslashes(input string) string {
	var b strings.Builder
	b.Grow(len(input))

	inString := false
	for i := 0; i < len(input); i++ {
		c := input[i]

		if !inString {
			if c == '"' {
				inString = true
			}
			b.WriteByte(c)
			continue
		}

		switch c {
		case '"':
			inString = false
			b.WriteByte(c)
		case '\\':
			if i+1 < len(input) && isJSONEscape(input[i+1]) {
				// 正しいエスケープはそのまま通します。ここで 2 バイト進めるのが要点で、
				// `\\` を 1 バイトずつ見ると 2 つ目を裸のバックスラッシュと誤認します。
				b.WriteByte(c)
				b.WriteByte(input[i+1])
				i++
				continue
			}
			// 続きが無効なエスケープ（あるいは末尾）なので、literal として二重にします。
			b.WriteString(`\\`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// escapeControlChars は、文字列リテラルの中に生のまま置かれた制御文字を
// エスケープ表記へ置き換えます。
//
// JSON は文字列の中に U+0020 未満の文字をそのまま置くことを許しません。にもかかわらず
// モデルは、**台本の抜粋や歌詞のように複数行の本文を載せるとき、生の改行・タブを
// 入れてきます。** 構造化出力を指定していても起こります。
//
// 置き換えは一意です。生の制御文字はその位置に現れてはいけない文字なので、エスケープ表記へ
// 写しても意味は変わりません。文字列の外は触りません（JSON の整形に使われた改行や
// インデントを壊さないためです）。
func escapeControlChars(input string) string {
	var b strings.Builder
	b.Grow(len(input))

	inString := false
	for i := 0; i < len(input); i++ {
		c := input[i]

		if !inString {
			if c == '"' {
				inString = true
			}
			b.WriteByte(c)
			continue
		}

		switch {
		case c == '"':
			inString = false
			b.WriteByte(c)
		case c == '\\':
			// 正しいエスケープは 2 バイトまとめて通します。1 バイトずつ見ると、
			// `\"` の引用符を文字列の終わりと誤認して以降の判定がずれます。
			b.WriteByte(c)
			if i+1 < len(input) && isJSONEscape(input[i+1]) {
				b.WriteByte(input[i+1])
				i++
			}
		case c < 0x20:
			b.WriteString(controlEscape(c))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// controlEscape は、制御文字に対する JSON のエスケープ表記を返します。
// 短い表記があるものはそちらを使います。ログや成果物を人が読むことがあるので、
// 改行が 6 桁の \u エスケープで並ぶより `\n` の方が読めます。
func controlEscape(c byte) string {
	switch c {
	case '\n':
		return `\n`
	case '\r':
		return `\r`
	case '\t':
		return `\t`
	case '\b':
		return `\b`
	case '\f':
		return `\f`
	default:
		return fmt.Sprintf(`\u%04x`, c)
	}
}

// isJSONEscape は、バックスラッシュの直後に置ける文字かどうかを返します。
//
// \u は続く 4 桁が 16 進である必要がありますが、そこまでは見ません。壊れた \u を
// 直す一意な方法が無く、触ると別の壊し方をするだけだからです。
func isJSONEscape(c byte) bool {
	switch c {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't', 'u':
		return true
	default:
		return false
	}
}
