package utils

import (
	"context"

	"go.uber.org/fx"
)

type hooks interface {
	Start(context.Context) error
	Stop(context.Context) error
}

func ToHook(c hooks) fx.Hook {
	return fx.Hook{
		OnStart: c.Start,
		OnStop:  c.Stop,
	}
}
