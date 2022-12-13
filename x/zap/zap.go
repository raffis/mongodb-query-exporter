package zap

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	// Encoding sets the logger's encoding. Valid values are "json" and
	// "console", as well as any third-party encodings registered via
	// RegisterEncoder.
	Encoding string `json:"encoding" yaml:"encoding"`
	// Level is the minimum enabled logging level. Note that this is a dynamic
	// level, so calling Config.Level.SetLevel will atomically change the log
	// level of all loggers descended from this config.
	Level string `json:"level" yaml:"level"`
	// Development puts the logger in development mode, which changes the
	// behavior of DPanicLevel and takes stacktraces more liberally.
	Development bool `json:"development" yaml:"development"`
	// DisableCaller stops annotating logs with the calling function's file
	// name and line number. By default, all logs are annotated.
	DisableCaller bool `json:"disableCaller" yaml:"disableCaller"`
}

// Initialize default configz
func NewConfig() Config {
	return Config{
		Level:    "info",
		Encoding: "json",
	}
}

// Initializes zap logger with environment variable configuration LOG_LEVEL and LOG_FORMAT.
func New(config Config) (*zap.Logger, error) {
	var c zap.Config
	if config.Development {
		c = zap.NewDevelopmentConfig()
		c.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		c = zap.NewProductionConfig()
	}

	level := zap.NewAtomicLevel()
	err := level.UnmarshalText([]byte(config.Level))

	if err != nil {
		return nil, err
	}

	c.DisableStacktrace = true
	c.Encoding = config.Encoding
	c.Development = config.Development
	c.DisableCaller = config.DisableCaller
	c.Level = level

	logger, err := c.Build()
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = logger.Sync()
	}()

	return logger, nil
}
