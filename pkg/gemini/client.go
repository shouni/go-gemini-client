package gemini

import (
	"context"
	"errors"
	"fmt"

	"github.com/shouni/go-utils/retry"
	"google.golang.org/genai"
)

// Package-level sentinel errors for robust error handling.
var (
	ErrEmptyPrompt        = errors.New("prompt cannot be empty")
	ErrAPIKeyRequired     = errors.New("API key is required")
	ErrInvalidTemperature = errors.New("temperature must be between 0.0 and 1.0")
)

// NewClient creates a new Gemini client based on the provided configuration.
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
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	temp := DefaultTemperature
	if cfg.Temperature != nil {
		if *cfg.Temperature < 0.0 || *cfg.Temperature > 1.0 {
			return nil, fmt.Errorf("%w, got: %f", ErrInvalidTemperature, *cfg.Temperature)
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

// GenerateContent generates content from a pure text prompt.
// This function applies default generation parameters such as TopP and CandidateCount.
// For more detailed control over generation options, use GenerateWithParts.
func (c *Client) GenerateContent(ctx context.Context, modelName string, prompt string) (*Response, error) {
	if prompt == "" {
		return nil, ErrEmptyPrompt
	}
	parts := []*genai.Part{{Text: prompt}}
	return c.GenerateWithParts(ctx, modelName, parts, GenerateOptions{})
}

// GenerateWithParts generates content by processing multimodal parts (text, images, etc.).
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

// generate encapsulates common API calling and retry logic.
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

	err := retry.Do(ctx, c.retryConfig, fmt.Sprintf("Gemini API call to %s", modelName), op, shouldRetry)
	if err != nil {
		return nil, err
	}

	return finalResp, nil
}
