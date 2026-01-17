package gemini

import (
	"context"
	"errors"
	"fmt"

	"github.com/shouni/go-utils/retry"
	"google.golang.org/genai"
)

// NewClient は設定を基に新しい Gemini クライアントを生成する。
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("APIキーは必須です。設定を確認してください")
	}

	clientConfig := &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	}

	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("Geminiクライアントの作成に失敗しました: %w", err)
	}

	temp := DefaultTemperature
	if cfg.Temperature != nil {
		if *cfg.Temperature < 0.0 || *cfg.Temperature > 1.0 {
			return nil, fmt.Errorf("温度設定は0.0から1.0の間である必要があります。入力値: %f", *cfg.Temperature)
		}
		temp = *cfg.Temperature
	}

	retryCfg := retry.DefaultConfig()
	if cfg.MaxRetries > 0 {
		retryCfg.MaxRetries = cfg.MaxRetries
	} else {
		retryCfg.MaxRetries = uint64(DefaultMaxRetries)
	}

	retryCfg.InitialInterval = DefaultInitialDelay
	if cfg.InitialDelay > 0 {
		retryCfg.InitialInterval = cfg.InitialDelay
	}

	retryCfg.MaxInterval = DefaultMaxDelay
	if cfg.MaxDelay > 0 {
		retryCfg.MaxInterval = cfg.MaxDelay
	}

	return &Client{
		client:      client,
		temperature: temp,
		retryConfig: retryCfg,
	}, nil
}

// GenerateWithParts はマルチモーダルパーツ（テキストや参照画像など）を処理してコンテンツを生成する。
func (c *Client) GenerateWithParts(ctx context.Context, modelName string, parts []*genai.Part, opts GenerateOptions) (*Response, error) {
	// --- AIへのリクエスト組み立て ---
	contents := []*genai.Content{{Role: "user", Parts: parts}}
	genConfig := &genai.GenerateContentConfig{
		Temperature:    genai.Ptr(c.temperature),
		TopP:           genai.Ptr(DefaultTopP),
		CandidateCount: DefaultCandidateCount,
		SafetySettings: opts.SafetySettings,
	}

	// シード値がある場合はキャストしてセットするのだ
	if opts.Seed != nil {
		genConfig.Seed = genai.Ptr(int32(*opts.Seed))
	}

	if opts.SystemPrompt != "" {
		genConfig.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: opts.SystemPrompt}},
		}
	}

	if opts.AspectRatio != "" {
		genConfig.ImageConfig = &genai.ImageConfig{AspectRatio: opts.AspectRatio}
	}

	var finalResp *Response
	op := func() error {
		resp, err := c.client.Models.GenerateContent(ctx, modelName, contents, genConfig)
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

	// 指数バックオフ付きのリトライ実行なのだ
	err := c.executeWithRetry(ctx, fmt.Sprintf("Gemini API call to %s", modelName), op, shouldRetry)
	if err != nil {
		return nil, err
	}

	return finalResp, nil
}

// GenerateContent は純粋なテキストプロンプトからコンテンツを生成する。
func (c *Client) GenerateContent(ctx context.Context, modelName string, finalPrompt string) (*Response, error) {
	if finalPrompt == "" {
		return nil, errors.New("プロンプトが空です。入力を確認してください")
	}

	var finalResp *Response
	contents := promptToContents(finalPrompt)
	config := &genai.GenerateContentConfig{
		Temperature: genai.Ptr(c.temperature),
	}

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

	err := c.executeWithRetry(ctx, fmt.Sprintf("Gemini API call to %s", modelName), op, shouldRetry)
	if err != nil {
		return nil, err
	}

	return finalResp, nil
}

// executeWithRetry は指定された操作をリトライ設定に従って実行する。
func (c *Client) executeWithRetry(ctx context.Context, operationName string, op func() error, shouldRetryFn func(error) bool) error {
	return retry.Do(ctx, c.retryConfig, operationName, op, shouldRetryFn)
}
