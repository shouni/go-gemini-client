package gemini

import (
	"context"
	"errors"
	"fmt"

	"github.com/shouni/go-utils/retry"
	"google.golang.org/genai"
)

// 堅牢なエラーハンドリングのためのパッケージレベルのセンチネルエラー。
var (
	ErrEmptyPrompt        = errors.New("プロンプトを空にすることはできません")
	ErrAPIKeyRequired     = errors.New("APIキーは必須です")
	ErrInvalidTemperature = errors.New("温度設定（Temperature）は 0.0 から 1.0 の間である必要があります")
)

// NewClient は提供された設定に基づいて、新しい Gemini クライアントを作成します。
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, ErrAPIKeyRequired
	}

	clientConfig := &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	}

	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("Gemini クライアントの作成に失敗しました: %w", err)
	}

	temp := DefaultTemperature
	if cfg.Temperature != nil {
		if *cfg.Temperature < 0.0 || *cfg.Temperature > 1.0 {
			return nil, fmt.Errorf("%w（入力値: %f）", ErrInvalidTemperature, *cfg.Temperature)
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
	retryCfg.MaxInterval = DefaultMaxDelay
	if cfg.InitialDelay > 0 {
		retryCfg.InitialInterval = cfg.InitialDelay
	}
	if cfg.MaxDelay > 0 {
		retryCfg.MaxInterval = cfg.MaxDelay
	}

	return &Client{
		client:      client,
		temperature: temp,
		retryConfig: retryCfg,
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
