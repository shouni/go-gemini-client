package gemini

import (
	"context"

	"google.golang.org/genai"
)

// GenerativeModel インターフェース
// Client がこれを満たすように実装します
type GenerativeModel interface {
	GenerateContent(ctx context.Context, modelName string, prompt string) (*Response, error)
	GenerateWithParts(ctx context.Context, modelName string, parts []*genai.Part, opts GenerateOptions) (*Response, error)
	UploadFile(ctx context.Context, data []byte, mimeType, displayName string) (string, string, error)
	DeleteFile(ctx context.Context, fileName string) error
}
