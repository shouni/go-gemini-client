package gemini

import (
	"errors"
	"fmt"
	"time"

	"google.golang.org/genai"
)

var (
	ErrConfigRequired         = errors.New("APIKey または ProjectID/LocationID のいずれかが必須です")
	ErrExclusiveConfig        = errors.New("ProjectID/LocationID と APIKey は排他的に設定してください")
	ErrIncompleteVertexConfig = errors.New("Vertex AIを使用する場合、ProjectIDとLocationIDの両方を設定してください")
	ErrInvalidTemperature     = errors.New("温度設定（Temperature）は 0.0 から 2.0 の間である必要があります")
)

// Config は初期化用の設定です。
// Vertex AI を使用する場合は ProjectID と LocationID を指定してください。
// Gemini API (Google AI Studio) を使用する場合は APIKey を指定してください。
type Config struct {
	APIKey       string
	ProjectID    string // Vertex AI: Google Cloud Project ID
	LocationID   string // Vertex AI: Location (e.g., "us-central1")
	Temperature  *float32
	MaxRetries   uint64
	InitialDelay time.Duration
	MaxDelay     time.Duration
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

	// 4. 数値バリデーション (Temperature 等)
	if err := c.validateTemperature(); err != nil {
		return err
	}

	return nil
}

// validateTemperature は Temperature の値が許容範囲内にあるかのみを検証します。
func (c Config) validateTemperature() error {
	if c.Temperature == nil {
		return nil
	}
	val := *c.Temperature
	if val < 0.0 || val > 2.0 {
		return fmt.Errorf("%w (入力値: %f)", ErrInvalidTemperature, val)
	}
	return nil
}

// getTemperature は検証済みの Temperature またはデフォルト値を返します。
func (c Config) getTemperature() float32 {
	if c.Temperature == nil {
		return DefaultTemperature
	}
	return *c.Temperature
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
