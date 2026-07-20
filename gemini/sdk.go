package gemini

import (
	"context"
	"io"
	"iter"

	"google.golang.org/genai"
)

type modelClient interface {
	GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
	GenerateContentStream(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) iter.Seq2[*genai.GenerateContentResponse, error]
	CountTokens(ctx context.Context, model string, contents []*genai.Content, config *genai.CountTokensConfig) (*genai.CountTokensResponse, error)
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

func (c genAIModelClient) GenerateContentStream(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) iter.Seq2[*genai.GenerateContentResponse, error] {
	return c.models.GenerateContentStream(ctx, model, contents, config)
}

func (c genAIModelClient) CountTokens(ctx context.Context, model string, contents []*genai.Content, config *genai.CountTokensConfig) (*genai.CountTokensResponse, error) {
	return c.models.CountTokens(ctx, model, contents, config)
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
