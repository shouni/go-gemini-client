// Package gemini は、Gemini API / Vertex AI 向けの genai SDK をラップし、
// リトライやFile APIアップロードを備えたクライアントを提供します。
package gemini

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shouni/netarmor/retry"
	"google.golang.org/genai"
)

// Client は Gemini SDK をラップしたメイン構造体です。
type Client struct {
	modelClient         modelClient
	fileClient          fileClient
	backend             genai.Backend
	retryConfig         retry.Config
	filePollingInterval time.Duration
	filePollingTimeout  time.Duration
}

// NewClient は提供された設定に基づいて、新しい Gemini クライアントを作成します。
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	clientCfg := cfg.toClientConfig()
	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("gemini: クライアントの作成に失敗しました: %w", err)
	}

	return &Client{
		modelClient:         genAIModelClient{models: client.Models},
		fileClient:          genAIFileClient{files: client.Files},
		backend:             clientCfg.Backend,
		retryConfig:         cfg.buildRetryConfig(),
		filePollingInterval: cfg.getFilePollingInterval(),
		filePollingTimeout:  cfg.getFilePollingTimeout(),
	}, nil
}

// IsVertexAI は、このクライアントが Vertex AI バックエンドを使用しているかを確認します。
func (c *Client) IsVertexAI() bool {
	return c.backend == genai.BackendVertexAI
}

// GenerateContent は純粋なテキストプロンプトからコンテンツを生成します。
func (c *Client) GenerateContent(ctx context.Context, modelName string, prompt string) (*Response, error) {
	if prompt == "" {
		return nil, ErrEmptyPrompt
	}
	parts := []*genai.Part{{Text: prompt}}
	return c.GenerateWithParts(ctx, modelName, parts, GenerateOptions{})
}

// GenerateWithParts はテキストや画像 (GCS URI含む) などのマルチモーダルパーツを処理してコンテンツを生成します。
func (c *Client) GenerateWithParts(ctx context.Context, modelName string, parts []*genai.Part, opts GenerateOptions) (*Response, error) {
	if err := validateGenerateInput(modelName, parts); err != nil {
		return nil, err
	}

	contents := []*genai.Content{{Role: "user", Parts: parts}}

	genConfig, err := c.buildGenerateConfig(opts)
	if err != nil {
		return nil, err
	}

	return c.generate(ctx, modelName, contents, genConfig)
}

func validateGenerateInput(modelName string, parts []*genai.Part) error {
	if modelName == "" {
		return ErrEmptyModelName
	}
	if len(parts) == 0 {
		return ErrEmptyParts
	}
	for _, part := range parts {
		if part == nil {
			return ErrInvalidPart
		}
	}
	return nil
}

func (c *Client) buildGenerateConfig(opts GenerateOptions) (*genai.GenerateContentConfig, error) {
	genConfig := &genai.GenerateContentConfig{
		SafetySettings: opts.SafetySettings,
	}

	if opts.ResponseMIMEType != "" {
		genConfig.ResponseMIMEType = opts.ResponseMIMEType

		if strings.HasPrefix(opts.ResponseMIMEType, "audio/") {
			genConfig.ResponseModalities = []string{"AUDIO"}
		} else if strings.HasPrefix(opts.ResponseMIMEType, "image/") {
			genConfig.ResponseModalities = []string{"IMAGE"}
		}
	}
	if opts.ResponseSchema != nil {
		genConfig.ResponseSchema = opts.ResponseSchema
	}
	if opts.Seed != nil {
		seed, err := seedToPtrInt32(opts.Seed)
		if err != nil {
			return nil, err
		}
		genConfig.Seed = seed
	}
	if opts.SystemPrompt != "" {
		genConfig.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: opts.SystemPrompt}},
		}
	}
	// 画像生成 (Imagen/Nano Banana) 用の設定
	if opts.HasImageConfig() {
		genConfig.ImageConfig = &genai.ImageConfig{}

		if len(genConfig.ResponseModalities) == 0 {
			genConfig.ResponseModalities = []string{"IMAGE"}
		}

		if opts.AspectRatio != "" {
			genConfig.ImageConfig.AspectRatio = opts.AspectRatio
		}
		if opts.ImageSize != "" {
			genConfig.ImageConfig.ImageSize = opts.ImageSize
		}
		if c.IsVertexAI() && opts.PersonGeneration != PersonGenerationUnspecified {
			genConfig.ImageConfig.PersonGeneration = string(opts.PersonGeneration)
		}
	}

	return genConfig, nil
}

// generate は共通の API 呼び出しとリトライロジックをカプセル化します。
func (c *Client) generate(ctx context.Context, modelName string, contents []*genai.Content, config *genai.GenerateContentConfig) (*Response, error) {
	var finalResp *Response

	op := func() error {
		resp, err := c.modelClient.GenerateContent(ctx, modelName, contents, config)
		if err != nil {
			return err
		}

		// レスポンスからテキストと画像を抽出
		text, extractErr := extractTextFromResponse(resp)
		if extractErr != nil {
			return extractErr
		}

		var images [][]byte
		var audios [][]byte
		if len(resp.Candidates) > 0 && resp.Candidates[0] != nil && resp.Candidates[0].Content != nil {
			for _, part := range resp.Candidates[0].Content.Parts {
				if part.InlineData != nil {
					mime := part.InlineData.MIMEType
					data := part.InlineData.Data

					// MIMEタイプで振り分け
					if strings.HasPrefix(mime, "image/") {
						images = append(images, data)
					} else if strings.HasPrefix(mime, "audio/") {
						audios = append(audios, data)
					}
				}
			}
		}

		finalResp = &Response{
			Text:        text,
			Images:      images,
			Audios:      audios,
			RawResponse: resp,
		}
		return nil
	}

	err := retry.Do(ctx, c.retryConfig, fmt.Sprintf("Gemini API 呼び出し（モデル: %s）", modelName), op, shouldRetry)
	if err != nil {
		return nil, err
	}

	return finalResp, nil
}
