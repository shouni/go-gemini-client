package gemini

import (
	"context"
	"fmt"

	"github.com/shouni/netarmor/retry"
	"google.golang.org/genai"
)

// NewClient は提供された設定に基づいて、新しい Gemini クライアントを作成します。
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	clientCfg := &genai.ClientConfig{}

	// 設定の有無を確認
	hasVertex := cfg.ProjectID != "" || cfg.LocationID != ""
	isVertexComplete := cfg.ProjectID != "" && cfg.LocationID != ""
	isGemini := cfg.APIKey != ""

	// 1. 排他制御のチェック
	if hasVertex && isGemini {
		return nil, ErrExclusiveConfig
	}

	// 2. 設定の完全性チェック
	if hasVertex && !isVertexComplete {
		return nil, ErrIncompleteVertexConfig
	}

	// 3. バックエンドの決定
	if isVertexComplete {
		// Vertex AI モード
		clientCfg.Project = cfg.ProjectID
		clientCfg.Location = cfg.LocationID
		clientCfg.Backend = genai.BackendVertexAI
	} else if isGemini {
		// Gemini API モード
		clientCfg.APIKey = cfg.APIKey
		clientCfg.Backend = genai.BackendGeminiAPI
	} else {
		return nil, ErrConfigRequired
	}

	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("Geminiクライアントの作成に失敗しました: %w", err)
	}

	// Temperature のバリデーションと設定
	temp, err := validateTemperature(cfg.Temperature)
	if err != nil {
		return nil, err
	}

	return &Client{
		client:      client,
		temperature: temp,
		retryConfig: buildRetryConfig(cfg),
		backend:     clientCfg.Backend,
	}, nil
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
	contents := []*genai.Content{{Role: "user", Parts: parts}}

	genConfig := &genai.GenerateContentConfig{
		Temperature:    genai.Ptr(c.temperature),
		TopP:           genai.Ptr(DefaultTopP),
		CandidateCount: DefaultCandidateCount,
		SafetySettings: opts.SafetySettings,
	}

	if opts.Seed != nil {
		genConfig.Seed = seedToPtrInt32(opts.Seed)
	}
	if opts.SystemPrompt != "" {
		genConfig.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: opts.SystemPrompt}},
		}
	}
	// 画像生成 (Imagen/Nano Banana) 用の設定
	if opts.AspectRatio != "" || opts.ImageSize != "" {
		genConfig.ImageConfig = &genai.ImageConfig{
			AspectRatio: opts.AspectRatio,
			ImageSize:   opts.ImageSize,
		}
	}

	return c.generate(ctx, modelName, contents, genConfig)
}

// generate は共通の API 呼び出しとリトライロジックをカプセル化します。
func (c *Client) generate(ctx context.Context, modelName string, contents []*genai.Content, config *genai.GenerateContentConfig) (*Response, error) {
	var finalResp *Response

	op := func() error {
		resp, err := c.client.Models.GenerateContent(ctx, modelName, contents, config)
		if err != nil {
			return err
		}

		// レスポンスからテキストと画像を抽出
		text, extractErr := extractTextFromResponse(resp)
		if extractErr != nil {
			return extractErr
		}

		var images [][]byte
		if len(resp.Candidates) > 0 && resp.Candidates[0] != nil && resp.Candidates[0].Content != nil {
			parts := resp.Candidates[0].Content.Parts
			for _, part := range parts {
				if part.InlineData != nil {
					images = append(images, part.InlineData.Data)
				}
			}
		}

		finalResp = &Response{
			Text:        text,
			Images:      images,
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
