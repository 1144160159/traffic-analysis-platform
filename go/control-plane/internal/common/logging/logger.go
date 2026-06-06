package logging

import (
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	Level       string `env:"LOG_LEVEL" envDefault:"info"`
	Format      string `env:"LOG_FORMAT" envDefault:"json"`
	Output      string `env:"LOG_OUTPUT" envDefault:"stdout"`
	Service     string `env:"SERVICE_NAME" envDefault:"unknown"`
	Version     string `env:"SERVICE_VERSION" envDefault:"unknown"`
	Environment string `env:"ENVIRONMENT" envDefault:"development"`

	SamplingInitial    int `env:"LOG_SAMPLING_INITIAL" envDefault:"100"`
	SamplingThereafter int `env:"LOG_SAMPLING_THEREAFTER" envDefault:"100"`
}

func NewLogger(cfg Config) (*zap.Logger, error) {
	level := parseLevel(cfg.Level)

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	if strings.ToLower(cfg.Format) == "console" {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	var writeSyncer zapcore.WriteSyncer
	switch strings.ToLower(cfg.Output) {
	case "stdout":
		writeSyncer = zapcore.AddSync(os.Stdout)
	case "stderr":
		writeSyncer = zapcore.AddSync(os.Stderr)
	default:
		file, err := os.OpenFile(cfg.Output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		writeSyncer = zapcore.AddSync(file)
	}

	core := zapcore.NewCore(encoder, writeSyncer, level)

	if cfg.SamplingInitial > 0 {
		core = zapcore.NewSamplerWithOptions(
			core,
			time.Second,
			cfg.SamplingInitial,
			cfg.SamplingThereafter,
		)
	}

	logger := zap.New(core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
		zap.Fields(
			zap.String(FieldService, cfg.Service),
			zap.String(FieldVersion, cfg.Version),
			zap.String(FieldEnvironment, cfg.Environment),
		),
	)

	zap.ReplaceGlobals(logger)

	return logger, nil
}

func NewDevelopmentLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

func NewProductionLogger(service, version string) *zap.Logger {
	cfg := Config{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		Service:     service,
		Version:     version,
		Environment: "production",
	}
	logger, _ := NewLogger(cfg)
	return logger
}

func parseLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "dpanic":
		return zapcore.DPanicLevel
	case "panic":
		return zapcore.PanicLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

func Sync(logger *zap.Logger) {
	_ = logger.Sync()
}

func With(logger *zap.Logger, fields ...zap.Field) *zap.Logger {
	return logger.With(fields...)
}

func WithError(logger *zap.Logger, err error) *zap.Logger {
	return logger.With(zap.Error(err))
}

func WithTenant(logger *zap.Logger, tenantID string) *zap.Logger {
	return logger.With(zap.String(FieldTenantID, tenantID))
}

func WithUser(logger *zap.Logger, userID, username string) *zap.Logger {
	return logger.With(
		zap.String(FieldUserID, userID),
		zap.String(FieldUsername, username),
	)
}

func WithTrace(logger *zap.Logger, traceID, spanID string) *zap.Logger {
	return logger.With(
		zap.String(FieldTraceID, traceID),
		zap.String(FieldSpanID, spanID),
	)
}

func WithRequest(logger *zap.Logger, requestID, method, path string) *zap.Logger {
	return logger.With(
		zap.String(FieldRequestID, requestID),
		zap.String(FieldMethod, method),
		zap.String(FieldPath, path),
	)
}
