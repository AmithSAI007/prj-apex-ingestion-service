package config

import (
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a configured zap.Logger based on the APP_ENV environment
// variable. In "production" mode it outputs structured JSON to stdout and a
// file. In all other modes it outputs colorized, human-readable console logs
// at Info level and above.
func NewLogger() (*zap.Logger, error) {
	var logger *zap.Logger
	var err error

	if os.Getenv("APP_ENV") != "local" {
		// Production: structured JSON format, writing to both stdout and app.log.
		cfg := zap.NewProductionConfig()
		cfg.OutputPaths = []string{"stdout", "app.log"}
		cfg.ErrorOutputPaths = []string{"stderr", "app.log"}
		cfg.EncoderConfig.LevelKey = "severity"
		cfg.EncoderConfig.EncodeLevel = func(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
			severity := strings.ToUpper(l.CapitalString())
			if severity == "WARN" {
				severity = "WARNING"
			}
			enc.AppendString(severity)
		}
		logger, err = cfg.Build()
	} else {
		// Development: colorized console output with RFC3339 timestamps for readability.
		config := zap.NewDevelopmentEncoderConfig()
		config.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339)
		config.EncodeLevel = zapcore.CapitalColorLevelEncoder

		logger = zap.New(zapcore.NewCore(
			zapcore.NewConsoleEncoder(config),
			zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout)),
			zap.InfoLevel,
		))
	}

	if err != nil {
		return nil, err
	}

	return logger, nil
}
