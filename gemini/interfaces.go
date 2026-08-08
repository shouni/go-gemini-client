package gemini

import (
	"context"
	"io"
)

// BackendInspector は、利用中のバックエンドを判定するインターフェースです。
//
// Vertex AI と Gemini API では、受け付ける安全設定の閾値や、GCS URI を直接参照できるかが
// 異なります。利用側がその差を吸収するために切り出しています。
type BackendInspector interface {
	// IsVertexAI は、Vertex AI バックエンドを使用しているかを返します。
	IsVertexAI() bool
}

// Generator は、テキストとバイナリ添付からコンテンツを生成するインターフェースです。
//
// genai の型を含まないため、利用側はこのインターフェースにだけ依存すれば
// genai SDK を import せずに済みます。モックも 1 メソッドで書けます。
type Generator interface {
	GenerateWithAttachments(ctx context.Context, modelName string, prompt string, attachments []Attachment, opts GenerateOptions) (*Response, error)
}

// VideoGenerator は、動画生成の長時間実行オペレーションを開始し進捗を確認する、
// 最小のインターフェースです。
//
// 完了までの待ち方（ポーリング間隔・タイムアウト・一時エラーの許容回数）は含めて
// いません。それはこの2つを呼ぶループ側の方針であり、実装を差し替える理由にならない
// ためです。veo パッケージがこのインターフェースを受け取ってループを組みます。
type VideoGenerator interface {
	StartVideo(ctx context.Context, modelName string, req VideoRequest) (*VideoOperation, error)
	PollVideo(ctx context.Context, operationName string) (*VideoOperation, error)
}

// FileManager は、Gemini API で使用するファイルのアップロードおよび管理を担います。
type FileManager interface {
	UploadFile(ctx context.Context, r io.Reader, mimeType, displayName string) (UploadedFile, error)
	DeleteFile(ctx context.Context, name string) error
}

// Model は、添付付き生成・ファイル管理・バックエンド判定を集約したインターフェースです。
//
// 参照画像をアップロードしてから添付として渡すような、生成とファイル管理の両方を
// 使う利用側がこれに依存します。
type Model interface {
	Generator
	FileManager
	BackendInspector
}
