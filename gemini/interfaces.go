package gemini

import (
	"context"
	"io"
	"iter"

	"google.golang.org/genai"
)

// ContentGenerator は、テキストやメッセージの生成に特化した最小のインターフェースです。
type ContentGenerator interface {
	GenerateContent(ctx context.Context, modelName string, prompt string) (*Response, error)
}

// Generator は、コンテンツ生成機能を担うインターフェースです。
type Generator interface {
	GenerateWithParts(ctx context.Context, modelName string, parts []*genai.Part, opts GenerateOptions) (*Response, error)
	IsVertexAI() bool
}

// MultimodalGenerator は、テキストとバイナリ添付からコンテンツを生成するインターフェースです。
//
// Generator と違い genai の型を含まないため、利用側はこのインターフェースにだけ依存すれば
// genai SDK を import せずに済みます。モックも 1 メソッドで書けます。マルチモーダル入力を
// Part 単位で細かく組み立てる必要がある場合は Generator を使ってください。
type MultimodalGenerator interface {
	GenerateWithAttachments(ctx context.Context, modelName string, prompt string, attachments []Attachment, opts GenerateOptions) (*Response, error)
}

// FileManager は、Gemini API で使用するファイルのアップロードおよび管理を担います。
type FileManager interface {
	UploadFile(ctx context.Context, r io.Reader, mimeType, displayName string) (UploadedFile, error)
	DeleteFile(ctx context.Context, name string) error
}

// GenerativeModel は、生成機能とファイル管理機能を集約したGeminiの操作用インターフェースです。
type GenerativeModel interface {
	Generator
	FileManager
}

// StreamGenerator は、ストリーミングでのコンテンツ生成を担うインターフェースです。
// Generator とは別インターフェースとして分離しており、既存の Generator 実装
// （テスト用モックを含む）に影響を与えずに利用側で必要に応じて実装できます。
type StreamGenerator interface {
	GenerateContentStream(ctx context.Context, modelName string, prompt string) (iter.Seq2[*Response, error], error)
	GenerateWithPartsStream(ctx context.Context, modelName string, parts []*genai.Part, opts GenerateOptions) (iter.Seq2[*Response, error], error)
}

// TokenCounter は、実際の生成を行わずにトークン数を見積もる機能を担うインターフェースです。
type TokenCounter interface {
	CountTokens(ctx context.Context, modelName string, prompt string) (int32, error)
	CountTokensWithParts(ctx context.Context, modelName string, parts []*genai.Part) (int32, error)
}
