package gemini

import (
	"context"
	"io"

	"google.golang.org/genai"
)

// ContentGenerator は、テキストやメッセージの生成に特化した最小のインターフェースです。
type ContentGenerator interface {
	GenerateContent(ctx context.Context, modelName string, prompt string) (*Response, error)
}

// Generator は、コンテンツ生成機能を担うインターフェースです。
type Generator interface {
	ContentGenerator
	GenerateWithParts(ctx context.Context, modelName string, parts []*genai.Part, opts GenerateOptions) (*Response, error)
	IsVertexAI() bool
}

// FileManager は、Gemini API で使用するファイルのアップロードおよび管理を担います。
type FileManager interface {
	UploadFile(ctx context.Context, r io.Reader, mimeType, displayName string) (string, string, error)
	DeleteFile(ctx context.Context, name string) error
}

// GenerativeModel は、生成機能とファイル管理機能を集約したGeminiの操作用インターフェースです。
type GenerativeModel interface {
	Generator
	FileManager
}
