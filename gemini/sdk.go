package gemini

import (
	"context"
	"io"

	"google.golang.org/genai"
)

type modelClient interface {
	GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
	EditImage(ctx context.Context, model string, prompt string, referenceImages []genai.ReferenceImage, config *genai.EditImageConfig) (*genai.EditImageResponse, error)
}

type fileClient interface {
	Upload(ctx context.Context, r io.Reader, config *genai.UploadFileConfig) (*genai.File, error)
	Get(ctx context.Context, name string, config *genai.GetFileConfig) (*genai.File, error)
	Delete(ctx context.Context, name string, config *genai.DeleteFileConfig) (*genai.DeleteFileResponse, error)
}

type genAIModelClient struct {
	models *genai.Models
}

func (c genAIModelClient) GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return c.models.GenerateContent(ctx, model, contents, config)
}

func (c genAIModelClient) EditImage(ctx context.Context, model string, prompt string, referenceImages []genai.ReferenceImage, config *genai.EditImageConfig) (*genai.EditImageResponse, error) {
	return c.models.EditImage(ctx, model, prompt, referenceImages, config)
}

type genAIFileClient struct {
	files *genai.Files
}

func (c genAIFileClient) Upload(ctx context.Context, r io.Reader, config *genai.UploadFileConfig) (*genai.File, error) {
	return c.files.Upload(ctx, r, config)
}

func (c genAIFileClient) Get(ctx context.Context, name string, config *genai.GetFileConfig) (*genai.File, error) {
	return c.files.Get(ctx, name, config)
}

func (c genAIFileClient) Delete(ctx context.Context, name string, config *genai.DeleteFileConfig) (*genai.DeleteFileResponse, error) {
	return c.files.Delete(ctx, name, config)
}
