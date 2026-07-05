package lyria

import "time"

type options struct {
	geminiModel      string
	lyriaModel       string
	rateInterval     time.Duration
	readingConverter ReadingConverter
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

// WithReadingConverter injects a custom converter to format text into reader-friendly phonetics.
func WithReadingConverter(converter ReadingConverter) Option {
	return func(opts *options) {
		opts.readingConverter = converter
	}
}

// WithRateInterval sets the interval used by the audio generation rate limiter.
func WithRateInterval(value time.Duration) Option {
	return func(opts *options) {
		opts.rateInterval = value
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
