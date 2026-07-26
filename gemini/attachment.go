package gemini

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// Attachment は、プロンプトに添えるバイナリ入力 1 件です。
//
// 画像・音声・PDF など、モデルへインラインで渡すデータを表します。genai.Part や genai.Blob を
// 組み立てる代わりにこれを使うことで、呼び出し側は genai SDK を import せずに済みます。
// マルチモーダル入力は「MIME type とバイト列」以上の情報を必要としないため、SDK の
// 型をそのまま外へ出す理由がありません。
type Attachment struct {
	// MIMEType は "image/png"、"audio/mpeg" のようなデータの種別です。必須です。
	MIMEType string
	// Data はインラインで送るバイト列です。空の要素は無視されます。
	Data []byte
}

// GenerateWithAttachments は、テキストプロンプトとバイナリ添付からコンテンツを生成します。
//
// GenerateWithParts の genai を伴わない入口です。テキスト 1 つと添付 n 件という、
// マルチモーダル呼び出しのほとんどを占める形に絞っています。GCS URI の参照や
// システム指示を Part 単位で細かく組み立てたい場合は GenerateWithParts を使ってください。
//
// prompt が空でも添付があれば送信します（音声や画像だけを渡して解析させる用途）。
// 両方が空の場合と、データを持つ添付に MIME type が無い場合はエラーを返します。
func (c *Client) GenerateWithAttachments(ctx context.Context, modelName string, prompt string, attachments []Attachment, opts GenerateOptions) (*Response, error) {
	parts, err := attachmentParts(prompt, attachments)
	if err != nil {
		return nil, err
	}
	return c.GenerateWithParts(ctx, modelName, parts, opts)
}

// attachmentParts は、プロンプトと添付を genai の Part スライスへ変換します。
//
// データが空の添付を落とすのは、画像を「あれば渡す」形で組み立てる呼び出し側が
// 空要素の除去を毎回書かずに済むようにするためです。落とした結果 Part が 1 つも
// 残らない場合は、空リクエストを送らずにエラーにします。
func attachmentParts(prompt string, attachments []Attachment) ([]*genai.Part, error) {
	parts := make([]*genai.Part, 0, len(attachments)+1)
	if prompt != "" {
		parts = append(parts, &genai.Part{Text: prompt})
	}

	for i, attachment := range attachments {
		if len(attachment.Data) == 0 {
			continue
		}
		if attachment.MIMEType == "" {
			return nil, fmt.Errorf("gemini: attachments[%d] にMIME typeが設定されていません", i)
		}
		parts = append(parts, &genai.Part{
			InlineData: &genai.Blob{MIMEType: attachment.MIMEType, Data: attachment.Data},
		})
	}

	if len(parts) == 0 {
		return nil, ErrEmptyParts
	}
	return parts, nil
}
