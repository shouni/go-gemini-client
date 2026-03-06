package gemini

import (
	"context"

	"google.golang.org/genai"
)

// GenerativeRunner は、コンテンツ生成機能を担うインターフェースです。
type GenerativeRunner interface {
	GenerateContent(ctx context.Context, modelName string, prompt string) (*Response, error)
	GenerateWithParts(ctx context.Context, modelName string, parts []*genai.Part, opts GenerateOptions) (*Response, error)
	IsVertexAI() bool
}

// FileRepository は、Gemini API で使用するファイルのアップロードおよび管理を担います。
type FileRepository interface {
	UploadFile(ctx context.Context, data []byte, mimeType, displayName string) (uri string, name string, err error)
	DeleteFile(ctx context.Context, fileName string) error
}

// ModelClient は、生成機能とファイル管理機能を集約したGeminiの操作用インターフェースです。
type ModelClient interface {
	GenerativeRunner
	FileRepository
}
