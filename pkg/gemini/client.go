package gemini

import (
	"context"
	"fmt"

	"github.com/shouni/netarmor/retry"
	"google.golang.org/genai"
)

// NewClient は提供された設定に基づいて、新しい Gemini クライアントを作成します。
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, ErrAPIKeyRequired
	}

	clientCfg := &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
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
	}, nil
}

// GenerateContent は純粋なテキストプロンプトからコンテンツを生成します。
// この関数では、TopP や CandidateCount などのデフォルトの生成パラメータが適用されます。
// より詳細な生成オプションを指定する場合は、GenerateWithParts を使用してください。
func (c *Client) GenerateContent(ctx context.Context, modelName string, prompt string) (*Response, error) {
	if prompt == "" {
		return nil, ErrEmptyPrompt
	}
	parts := []*genai.Part{{Text: prompt}}
	return c.GenerateWithParts(ctx, modelName, parts, GenerateOptions{})
}

// GenerateWithParts はテキストや画像などのマルチモーダルパーツを処理してコンテンツを生成します。
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
		text, extractErr := extractTextFromResponse(resp)
		if extractErr != nil {
			return extractErr
		}
		finalResp = &Response{Text: text, RawResponse: resp}
		return nil
	}

	err := retry.Do(ctx, c.retryConfig, fmt.Sprintf("Gemini API 呼び出し（モデル: %s）", modelName), op, shouldRetry)
	if err != nil {
		return nil, err
	}

	return finalResp, nil
}
