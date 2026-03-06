package gemini

import (
	"context"

	"google.golang.org/genai"
)

// GenerativeRunner 生成機能に特化したインターフェース
type GenerativeRunner interface {
	GenerateContent(ctx context.Context, modelName string, prompt string) (*Response, error)
	GenerateWithParts(ctx context.Context, modelName string, parts []*genai.Part, opts GenerateOptions) (*Response, error)
	IsVertexAI() bool
}

// FileRepository ファイル管理機能に特化したインターフェース
type FileRepository interface {
	UploadFile(ctx context.Context, data []byte, mimeType, displayName string) (string, string, error)
	DeleteFile(ctx context.Context, fileName string) error
}

// ModelService 上位インターフェースを定義する（Facadeパターン）
type ModelService interface {
	GenerativeRunner
	FileRepository
}
