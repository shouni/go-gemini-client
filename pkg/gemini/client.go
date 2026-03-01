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

	// 1. 排他制御 (Conflict Check)
	// Vertex AIの設定が一部でも存在し、かつAPIKeyもある場合は排他エラーを優先
	if (cfg.IsVertexAI() || cfg.IsIncompleteVertex()) && cfg.IsGeminiAPI() {
		return nil, ErrExclusiveConfig
	}

	// 2. 完全性チェック (Incomplete Vertex Check)
	// 片方だけ設定されている中途半端な状態を検知
	if cfg.IsIncompleteVertex() {
		return nil, ErrIncompleteVertexConfig
	}

	// 3. バックエンドの決定と必須チェック
	if cfg.IsVertexAI() {
		// Vertex AI モード
		clientCfg.Project = cfg.ProjectID
		clientCfg.Location = cfg.LocationID
		clientCfg.Backend = genai.BackendVertexAI
	} else if cfg.IsGeminiAPI() {
		// Gemini API モード
		clientCfg.APIKey = cfg.APIKey
		clientCfg.Backend = genai.BackendGeminiAPI
	} else {
		// どちらの条件も満たさない場合は必須エラー
		return nil, ErrConfigRequired
	}

	// クライアントの初期化
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
	if opts.AspectRatio != "" || opts.ImageSize != "" || opts.PersonGeneration != "" {
		genConfig.ImageConfig = &genai.ImageConfig{
			AspectRatio:      opts.AspectRatio,
			ImageSize:        opts.ImageSize,
			PersonGeneration: string(opts.PersonGeneration),
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
