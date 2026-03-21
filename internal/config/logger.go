package config

import (
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a configured zap.Logger based on the APP_ENV environment
// variable. In "production" mode it outputs structured JSON to stdout.
// In all other modes it outputs colorized, human-readable console logs
// at Info level and above.
//
// The serviceName parameter is added as a base field to every log entry,
// making it possible to distinguish logs from different services when they
// are routed to a shared sink (e.g., BigQuery).
func NewLogger(appEnv string, serviceName string) (*zap.Logger, error) {
	var logger *zap.Logger
	var err error

	if os.Getenv(appEnv) != "local" {
		// Production: structured JSON format to stdout only.
		// Cloud Run captures stdout; writing to ephemeral files is avoided.
		cfg := zap.NewProductionConfig()
		cfg.OutputPaths = []string{"stdout"}
		cfg.ErrorOutputPaths = []string{"stderr"}
		cfg.EncoderConfig.LevelKey = "severity"
		cfg.EncoderConfig.EncodeLevel = func(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
			severity := strings.ToUpper(l.CapitalString())
			if severity == "WARN" {
				severity = "WARNING"
			}
			enc.AppendString(severity)
		}
		logger, err = cfg.Build(
			zap.Fields(zap.String("service", serviceName)),
		)
	} else {
		// Development: colorized console output with RFC3339 timestamps for readability.
		config := zap.NewDevelopmentEncoderConfig()
		config.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339)
		config.EncodeLevel = zapcore.CapitalColorLevelEncoder

		logger = zap.New(zapcore.NewCore(
			zapcore.NewConsoleEncoder(config),
			zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout)),
			zap.InfoLevel,
		),
			zap.Fields(zap.String("service", serviceName)),
		)
	}

	if err != nil {
		return nil, err
	}

	return logger, nil
}
