package log

import (
	"context"
	"os"
	"time"

	"github.com/rs/zerolog"
)

var base zerolog.Logger

func Init(verbose bool) {
	output := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
	}

	level := zerolog.InfoLevel
	if verbose {
		level = zerolog.DebugLevel
	}

	base = zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Logger()
}

func WithContext(ctx context.Context) context.Context {
	return base.WithContext(ctx)
}

func FromContext(ctx context.Context) *zerolog.Logger {
	return zerolog.Ctx(ctx)
}
