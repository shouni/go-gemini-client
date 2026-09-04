package lyria

import "time"

type options struct {
	geminiModel      string
	lyriaModel       string
	rateInterval     time.Duration
	textRateInterval time.Duration
	execTimeout      time.Duration
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

// WithRateInterval sets the interval used by the audio generation rate limiter.
func WithRateInterval(value time.Duration) Option {
	return func(opts *options) {
		opts.rateInterval = value
	}
}

// WithTextRateInterval sets the interval used by the lyrics/recipe (text) generation rate limiter.
// Unset (zero value) means no rate limiting, matching the default behavior of WithRateInterval.
func WithTextRateInterval(value time.Duration) Option {
	return func(opts *options) {
		opts.textRateInterval = value
	}
}

// WithExecTimeout sets the per-execution timeout applied to singleflight-shared
// generation calls. The execution context is detached from the caller, so this is
// the only thing that bounds a shared run. Zero (unset) means callguard.DefaultExecTimeout.
//
// The rate-limit wait is not counted against it (see callguard.Do).
func WithExecTimeout(value time.Duration) Option {
	return func(opts *options) {
		opts.execTimeout = value
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
