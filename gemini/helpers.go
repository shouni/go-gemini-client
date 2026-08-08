package gemini

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strings"

	"google.golang.org/genai"
)

// shouldRetry は、発生したエラーがリトライによって解決可能かどうかを判定します。
func shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// 安全フィルターによるブロック等の論理エラーはリトライしても解決しないため即座に終了します。
	if _, ok := errors.AsType[*APIResponseError](err); ok {
		return false
	}

	// コンテキストのキャンセルやタイムアウト（呼び出し側管理）はリトライ対象外です。
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// genai SDK は REST で通信し、API エラーを HTTP ステータスコード付きの
	// genai.APIError（値型）として返すため、ステータスコードで判定します。
	if apiErr, ok := errors.AsType[genai.APIError](err); ok {
		switch apiErr.Code {
		case http.StatusTooManyRequests, // レート制限
			http.StatusInternalServerError, // サーバー内部エラー
			http.StatusServiceUnavailable,  // 一時的なサービス停止
			http.StatusGatewayTimeout:      // サーバー側でのタイムアウト
			return true
		default:
			return false
		}
	}

	// APIError に分類されないエラー（ネットワーク接続エラー、EOFなど）は一時的な障害の可能性があるためリトライを許可します。
	if errors.Is(err, io.EOF) {
		return true
	}
	if netErr, ok := errors.AsType[net.Error](err); ok {
		return netErr.Timeout()
	}

	return false
}

// firstCandidate は、レスポンスの先頭候補を返します。候補が無い場合は nil です。
//
// パートやコンテンツと同様、候補スロット自体もサーバーから来る値であり nil であり得ます。
// テキスト・思考・インラインデータの各抽出関数が個別に nil 判定を書くと、判定の
// 抜け漏れが関数ごとにずれるため（実際にずれていた）、走査の入口をここに集約しています。
func firstCandidate(resp *genai.GenerateContentResponse) *genai.Candidate {
	if resp == nil || len(resp.Candidates) == 0 {
		return nil
	}
	return resp.Candidates[0]
}

// candidateParts は、候補のパート列を返します。候補やコンテンツが nil の場合は nil です。
func candidateParts(candidate *genai.Candidate) []*genai.Part {
	if candidate == nil || candidate.Content == nil {
		return nil
	}
	return candidate.Content.Parts
}

// extractText は resp からテキストを抽出します。
func extractText(resp *genai.GenerateContentResponse) (string, error) {
	candidate := firstCandidate(resp)
	if candidate == nil {
		return "", newEmptyResponseError()
	}

	// FinishReason が正常（未設定 または STOP）以外の場合は、ブロックされたとみなします。
	// 未設定の判定は isUnsetFinishReason に集約しています（ゼロ値と SDK 定数が別値のため）。
	if isBlockedFinishReason(candidate.FinishReason) {
		return "", newBlockedError(candidate.FinishReason)
	}

	// すべてのテキストパートを連結して返します。
	//
	// 思考機能 (ThinkingBudget) が有効なモデルは、思考サマリを Thought=true の
	// テキストパートとして本文より前に返します。最初の非空テキストを返す実装では
	// 本文ではなく思考サマリを返してしまうため、Thought パートは除外します。
	// また、モデルは本文を複数パートに分割して返すことがあるため連結が必要です。
	var sb strings.Builder
	for _, part := range candidateParts(candidate) {
		if part == nil || part.Thought || part.Text == "" {
			continue
		}
		sb.WriteString(part.Text)
	}

	return sb.String(), nil
}

// extractThoughts は思考サマリ（Thought=true のテキストパート）を連結して返します。
// 思考機能が無効な場合や思考サマリが返されない場合は空文字列になります。
func extractThoughts(resp *genai.GenerateContentResponse) string {
	var sb strings.Builder
	for _, part := range candidateParts(firstCandidate(resp)) {
		if part == nil || !part.Thought || part.Text == "" {
			continue
		}
		sb.WriteString(part.Text)
	}
	return sb.String()
}

// extractInlineData は、レスポンスに含まれるインラインデータを MIME type 付きで
// 返却順のまま取り出します。
//
// nil のパートを読み飛ばすのは extractText / extractThoughts と同じ理由です。
// パートはサーバーから来るものなので、こちら側の検証で nil を排除できません。
func extractInlineData(resp *genai.GenerateContentResponse) []Attachment {
	var attachments []Attachment
	for _, part := range candidateParts(firstCandidate(resp)) {
		if part == nil || part.InlineData == nil {
			continue
		}
		attachments = append(attachments, Attachment{
			MIMEType: part.InlineData.MIMEType,
			Data:     part.InlineData.Data,
		})
	}
	return attachments
}

// tokenUsageFromMetadata は genai のトークン使用量メタデータを公開型に変換します。
func tokenUsageFromMetadata(meta *genai.GenerateContentResponseUsageMetadata) *TokenUsage {
	if meta == nil {
		return nil
	}
	return &TokenUsage{
		PromptTokenCount:     meta.PromptTokenCount,
		CandidatesTokenCount: meta.CandidatesTokenCount,
		TotalTokenCount:      meta.TotalTokenCount,
		ThoughtsTokenCount:   meta.ThoughtsTokenCount,
	}
}

// seedToPtrInt32 は *int64 を SDK 用の *int32 に変換します。
func seedToPtrInt32(s *int64) (*int32, error) {
	if s == nil {
		return nil, nil
	}

	if *s > math.MaxInt32 || *s < math.MinInt32 {
		return nil, fmt.Errorf("%w (入力値: %d)", ErrInvalidSeed, *s)
	}

	return new(int32(*s)), nil
}
