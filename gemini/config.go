package gemini

import (
	"errors"
	"time"

	"github.com/shouni/netarmor/retry"
	"google.golang.org/genai"
)

var (
	ErrConfigRequired         = errors.New("APIKey または ProjectID/LocationID のいずれかが必須です")
	ErrExclusiveConfig        = errors.New("ProjectID/LocationID と APIKey は排他的に設定してください")
	ErrIncompleteVertexConfig = errors.New("Vertex AIを使用する場合、ProjectIDとLocationIDの両方を設定してください")
)

// Config は初期化用の設定です。
// Vertex AI を使用する場合は ProjectID と LocationID を指定してください。
// Gemini API (Google AI Studio) を使用する場合は APIKey を指定してください。
type Config struct {
	APIKey              string
	ProjectID           string // Vertex AI: Google Cloud Project ID
	LocationID          string // Vertex AI: Location (e.g., "us-central1")
	MaxRetries          uint64
	InitialDelay        time.Duration
	MaxDelay            time.Duration
	FilePollingInterval time.Duration
	FilePollingTimeout  time.Duration
}

// isVertexAI ProjectIDおよびLocationIDのセットを確認し、Vertex AIの設定が有効であるかをチェックします。
func (c Config) isVertexAI() bool {
	return c.ProjectID != "" && c.LocationID != ""
}

// isGeminiAPI APIKeyの有無を検証し、Gemini APIを利用するための設定が有効であるかを確認します。
func (c Config) isGeminiAPI() bool {
	return c.APIKey != ""
}

// isIncompleteVertex ProjectIDまたはLocationIDの有無を確認し、Vertex AIの設定漏れがないかを検証します。
func (c Config) isIncompleteVertex() bool {
	hasAny := c.ProjectID != "" || c.LocationID != ""
	return hasAny && !c.isVertexAI()
}

// validate は設定内容が正しいか、排他制御や値の範囲をチェックします。
func (c Config) validate() error {
	// 1. 排他制御
	if (c.isVertexAI() || c.isIncompleteVertex()) && c.isGeminiAPI() {
		return ErrExclusiveConfig
	}

	// 2. 完全性チェック
	if c.isIncompleteVertex() {
		return ErrIncompleteVertexConfig
	}

	// 3. 必須チェック
	if !c.isVertexAI() && !c.isGeminiAPI() {
		return ErrConfigRequired
	}

	return nil
}

// toClientConfig Config を genai.ClientConfig に変換します。
func (c Config) toClientConfig() *genai.ClientConfig {
	cc := &genai.ClientConfig{}
	if c.isVertexAI() {
		cc.Project = c.ProjectID
		cc.Location = c.LocationID
		cc.Backend = genai.BackendVertexAI
	} else {
		cc.APIKey = c.APIKey
		cc.Backend = genai.BackendGeminiAPI
	}
	return cc
}

// buildRetryConfig は設定から retry.Config を構築します。
func (c Config) buildRetryConfig() retry.Config {
	rc := retry.DefaultConfig()

	if c.MaxRetries > 0 {
		rc.MaxRetries = c.MaxRetries
	} else {
		rc.MaxRetries = DefaultMaxRetries
	}

	if c.InitialDelay > 0 {
		rc.InitialInterval = c.InitialDelay
	} else {
		rc.InitialInterval = DefaultInitialDelay
	}

	if c.MaxDelay > 0 {
		rc.MaxInterval = c.MaxDelay
	} else {
		rc.MaxInterval = DefaultMaxDelay
	}

	return rc
}

func (c Config) getFilePollingInterval() time.Duration {
	if c.FilePollingInterval > 0 {
		return c.FilePollingInterval
	}
	return PollingInterval
}

func (c Config) getFilePollingTimeout() time.Duration {
	if c.FilePollingTimeout > 0 {
		return c.FilePollingTimeout
	}
	return PollingTimeout
}
