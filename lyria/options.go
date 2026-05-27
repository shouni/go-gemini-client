package lyria

import "time"

type options struct {
	geminiModel        string
	lyriaModel         string
	rateInterval       time.Duration
	maxConcurrency     int
	audioPromptBuilder AudioPromptBuilder
}

// Option configures Adapter.
type Option func(*options)

// WithGeminiModel sets the model used for lyrics and recipe generation.
func WithGeminiModel(value string) Option {
	return func(opts *options) {
		opts.geminiModel = value
	}
}

// WithLyriaModel sets the model used for audio generation.
func WithLyriaModel(value string) Option {
	return func(opts *options) {
		opts.lyriaModel = value
	}
}

// WithAudioPromptBuilder injects a custom builder for Lyria audio prompts.
func WithAudioPromptBuilder(builder AudioPromptBuilder) Option {
	return func(opts *options) {
		opts.audioPromptBuilder = builder
	}
}

// WithRateInterval sets the interval used by the audio generation rate limiter.
func WithRateInterval(value time.Duration) Option {
	return func(opts *options) {
		opts.rateInterval = value
	}
}

// WithMaxConcurrency sets the maximum concurrent section audio generations.
func WithMaxConcurrency(value int) Option {
	return func(opts *options) {
		opts.maxConcurrency = value
	}
}

func applyOptions(overrides ...Option) options {
	var opts options
	for _, override := range overrides {
		if override != nil {
			override(&opts)
		}
	}
	return opts
}
