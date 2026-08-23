// Package gemini は、Gemini API / Vertex AI 向けの genai SDK をラップし、
// リトライや File API アップロードを備えたクライアントを提供します。
//
// 公開 API に genai の型は現れません。設定値の型と定数は別名として再エクスポート
// してあるため（ThinkingLevel / SafetyThreshold / Schema / SchemaType /
// VideoReferenceType）、値を選ぶためだけに genai SDK を import する必要はありません。
// 別名なので、genai の値をそのまま渡しても構いません。
package gemini

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shouni/netarmor/retry"
	"google.golang.org/genai"
)

// Client がパッケージ公開インターフェースを満たすことをコンパイル時に保証します。
// これらのアサーションがないと、Client のメソッドシグネチャがドリフトしても
// 下流の利用側がビルドされるまで気付けません。
var (
	_ BackendInspector = (*Client)(nil)
	_ FileManager      = (*Client)(nil)
	_ Generator        = (*Client)(nil)
	_ Model            = (*Client)(nil)
	_ VideoGenerator   = (*Client)(nil)
)

// Client は Gemini SDK をラップしたメイン構造体です。
type Client struct {
	modelClient         modelClient
	fileClient          fileClient
	videoClient         videoClient
	backend             genai.Backend
	retryOpts           []retry.Option
	logger              *slog.Logger
	requestTimeout      time.Duration
	filePollingInterval time.Duration
	filePollingTimeout  time.Duration
	asyncCleanupTimeout time.Duration
}

// NewClient は提供された設定に基づいて、新しい Gemini クライアントを作成します。
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	clientCfg, err := cfg.toClientConfig()
	if err != nil {
		return nil, err
	}
	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("gemini: クライアントの作成に失敗しました: %w", err)
	}

	return &Client{
		modelClient:         genAIModelClient{models: client.Models},
		fileClient:          genAIFileClient{files: client.Files},
		videoClient:         genAIVideoClient{models: client.Models, operations: client.Operations},
		backend:             clientCfg.Backend,
		retryOpts:           cfg.buildRetryOptions(),
		logger:              cfg.getLogger(),
		requestTimeout:      cfg.RequestTimeout,
		filePollingInterval: cfg.getFilePollingInterval(),
		filePollingTimeout:  cfg.getFilePollingTimeout(),
		asyncCleanupTimeout: cfg.getAsyncCleanupTimeout(),
	}, nil
}

// IsVertexAI は、このクライアントが Vertex AI バックエンドを使用しているかを確認します。
func (c *Client) IsVertexAI() bool {
	return c.backend == genai.BackendVertexAI
}

// log は設定済みロガーを返します。NewClient を通らないゼロ値の Client
// （テストの構造体リテラルなど）でも安全に動くようフォールバックを持ちます。
func (c *Client) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	return slog.Default()
}

// requestContext は Config.RequestTimeout を適用した context を返します。
// 未設定（0）の場合は呼び出し元の context をそのまま使います。
func (c *Client) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.requestTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.requestTimeout)
}

// GenerateContent は純粋なテキストプロンプトからコンテンツを生成します。
func (c *Client) GenerateContent(ctx context.Context, modelName string, prompt string) (*Response, error) {
	if prompt == "" {
		return nil, ErrEmptyPrompt
	}
	parts := []*genai.Part{{Text: prompt}}
	return c.generateParts(ctx, modelName, parts, GenerateOptions{})
}

// generateParts はマルチモーダルパーツからコンテンツを生成する共通経路です。
//
// 公開の入口は genai の型を伴わない GenerateContent / GenerateWithAttachments で、
// genai.Part を直接受けるこの関数は公開しません（SDK の型を公開面に漏らさないため）。
func (c *Client) generateParts(ctx context.Context, modelName string, parts []*genai.Part, opts GenerateOptions) (*Response, error) {
	if err := validateGenerateInput(modelName, parts); err != nil {
		return nil, err
	}

	contents := []*genai.Content{{Role: "user", Parts: parts}}

	genConfig, err := buildGenerateConfig(opts, c.IsVertexAI())
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

// buildThinkingConfig は思考設定を組み立てます。
// 何も指定がなければ nil を返します。常に送るとモデル既定の思考挙動を上書きしてしまうためです。
//
// ThinkingLevel（段階指定）と ThinkingBudget（トークン数指定）は排他的な指定方法です。
// 両方が設定された場合は、モデル非依存で移植性の高い ThinkingLevel を優先します。
func buildThinkingConfig(opts GenerateOptions) *genai.ThinkingConfig {
	hasLevel := opts.ThinkingLevel != "" && opts.ThinkingLevel != genai.ThinkingLevelUnspecified
	if !hasLevel && opts.ThinkingBudget == nil && !opts.IncludeThoughts {
		return nil
	}

	cfg := &genai.ThinkingConfig{IncludeThoughts: opts.IncludeThoughts}
	if hasLevel {
		cfg.ThinkingLevel = opts.ThinkingLevel
		return cfg
	}
	cfg.ThinkingBudget = opts.ThinkingBudget
	return cfg
}

// applyResponseFormat は、レスポンスの MIME type・モダリティ・構造化出力スキーマを適用します。
func applyResponseFormat(genConfig *genai.GenerateContentConfig, opts GenerateOptions) {
	if opts.ResponseMIMEType != "" {
		genConfig.ResponseMIMEType = opts.ResponseMIMEType

		if strings.HasPrefix(opts.ResponseMIMEType, "audio/") {
			genConfig.ResponseModalities = []string{"AUDIO"}
		} else if strings.HasPrefix(opts.ResponseMIMEType, "image/") {
			genConfig.ResponseModalities = []string{"IMAGE"}
		}
	}
	// ResponseJSONSchema と ResponseSchema は排他。両方送るとどちらが効くか不定になるため、
	// 新しい ResponseJSONSchema を優先して片方だけ送る。
	switch {
	case opts.ResponseJSONSchema != nil:
		genConfig.ResponseJsonSchema = opts.ResponseJSONSchema
	case opts.ResponseSchema != nil:
		genConfig.ResponseSchema = opts.ResponseSchema
	}
}

// applyImageConfig は、画像生成 (Imagen/Nano Banana) 特有の設定を適用します。
// PersonGeneration は Vertex AI バックエンドでのみ送信します（Gemini API は未対応のため）。
func applyImageConfig(genConfig *genai.GenerateContentConfig, opts GenerateOptions, vertexAI bool) {
	if !opts.HasImageConfig() {
		return
	}
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
	if vertexAI && opts.PersonGeneration != PersonGenerationUnspecified {
		genConfig.ImageConfig.PersonGeneration = string(opts.PersonGeneration)
	}
}

// buildGenerateConfig は GenerateOptions を genai の生成設定へ変換します。
// Client のメソッドにしないのは、バックエンド判定（vertexAI）以外に Client の状態へ
// 依存しないためで、クライアント無しでテストできます。
func buildGenerateConfig(opts GenerateOptions, vertexAI bool) (*genai.GenerateContentConfig, error) {
	genConfig := &genai.GenerateContentConfig{
		SafetySettings:  opts.SafetySettings,
		Temperature:     opts.Temperature,
		TopP:            opts.TopP,
		TopK:            opts.TopK,
		MaxOutputTokens: opts.MaxOutputTokens,
		StopSequences:   opts.StopSequences,
	}

	genConfig.ThinkingConfig = buildThinkingConfig(opts)
	applyResponseFormat(genConfig, opts)

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
	applyImageConfig(genConfig, opts, vertexAI)

	return genConfig, nil
}

// generate は共通の API 呼び出しとリトライロジックをカプセル化します。
// Config.RequestTimeout が設定されている場合、リトライを含む呼び出し全体に適用されます。
func (c *Client) generate(ctx context.Context, modelName string, contents []*genai.Content, config *genai.GenerateContentConfig) (*Response, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	return runWithRetry(ctx, c.retryOpts,
		fmt.Sprintf("Gemini API 呼び出し（モデル: %s）", modelName),
		func() (*Response, error) {
			resp, err := c.modelClient.GenerateContent(ctx, modelName, contents, config)
			if err != nil {
				return nil, err
			}
			return responseFromGenAI(resp)
		})
}

// responseFromGenAI は genai のレスポンスをパッケージ公開型の Response に変換します。
func responseFromGenAI(resp *genai.GenerateContentResponse) (*Response, error) {
	text, err := extractText(resp)
	if err != nil {
		return nil, err
	}

	// MIME type で振り分ける。Images / Audios は Attachments の部分集合で、
	// 型を意識せずバイト列だけ欲しい呼び出し側のための入口です。
	attachments := extractInlineData(resp)
	var images [][]byte
	var audios [][]byte
	for _, attachment := range attachments {
		switch {
		case strings.HasPrefix(attachment.MIMEType, "image/"):
			images = append(images, attachment.Data)
		case strings.HasPrefix(attachment.MIMEType, "audio/"):
			audios = append(audios, attachment.Data)
		}
	}

	return &Response{
		Text:        text,
		Images:      images,
		Audios:      audios,
		Attachments: attachments,
		Thoughts:    extractThoughts(resp),
		Usage:       tokenUsageFromMetadata(resp.UsageMetadata),
	}, nil
}
