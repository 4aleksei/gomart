package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type (
	ZapLogger struct {
		Logger *zap.Logger
		level  zap.AtomicLevel
	}

	Config struct {
		Level string
	}
)

func New(cfg Config) (*ZapLogger, error) {
	var level zapcore.Level
	err := level.Set(cfg.Level)
	if err != nil {
		return nil, err
	}

	atomic := zap.NewAtomicLevelAt(level)
	settings := defaultSettings(atomic)

	l, err := settings.config.Build(settings.opts...)
	if err != nil {
		return nil, err
	}

	return &ZapLogger{
		Logger: l,
		level:  atomic,
	}, nil
}

func (z *ZapLogger) SetLevel(cfg Config) error {
	var level zapcore.Level
	err := level.Set(cfg.Level)
	if err != nil {
		return err
	}
	z.level.SetLevel(level)
	return nil
}

func (z *ZapLogger) Start(ctx context.Context) error {
	return nil
}

func (z *ZapLogger) Stop(ctx context.Context) error {
	_ = z.Logger.Sync()
	return nil
}
