package main

import (
	"time"

	"github.com/4aleksei/gmart/internal/common/logger"
	"github.com/4aleksei/gmart/internal/common/store"

	"github.com/4aleksei/gmart/internal/common/httpclientpool"
	"github.com/4aleksei/gmart/internal/common/store/pg"
	"github.com/4aleksei/gmart/internal/common/utils"
	"github.com/4aleksei/gmart/internal/gophermart/accrual"
	"github.com/4aleksei/gmart/internal/gophermart/config"
	"github.com/4aleksei/gmart/internal/gophermart/handlers"
	"github.com/4aleksei/gmart/internal/gophermart/service"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

func main() {
	setupFX().Run()
}

func setupFX() *fx.App {
	app := fx.New(
		fx.Supply(logger.Config{Level: "debug"}),
		fx.StopTimeout(1*time.Minute),
		fx.Provide(
			logger.New,
			config.GetConfig,
			fx.Annotate(pg.New,
				fx.As(new(service.ServiceStore)), fx.As(new(store.Store))),

			httpclientpool.NewHandler,
			service.NewService,
			handlers.NewHTTPServer,
			accrual.NewAccrual,
		),

		fx.WithLogger(func(log *logger.ZapLogger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: log.Logger}
		}),
		fx.Invoke(
			registerSetLoggerLevel,

			gooseUP,

			registerStorePg,
			registerHTTPClientPool,
			registerAccrualClient,
			registerHTTPServer,
		),
	)
	return app
}

func gooseUP(cfg *config.Config, ll *logger.ZapLogger) {
	if err := migrate(cfg.DatabaseURI, ll); err != nil {
		ll.Logger.Fatal("migrate fatal", zap.Error(err))
	}
}

func registerHTTPClientPool(h *httpclientpool.PoolHandler, cfg *config.Config) {
	h.SetCfgInit(uint64(cfg.RateLimit), cfg.AccrualSystemAddress)
}

func registerStorePg(ss store.Store, cfg *config.Config, lc fx.Lifecycle) {
	switch v := ss.(type) {
	case *pg.PgStore:
		v.DatabaseURI = cfg.DatabaseURI
		lc.Append(utils.ToHook(v))
	default:
	}
}

func registerAccrualClient(hh *accrual.HandlersAccrual, lc fx.Lifecycle) {
	lc.Append(utils.ToHook(hh))
}

func registerHTTPServer(hh *handlers.HandlersServer, lc fx.Lifecycle) {
	lc.Append(utils.ToHook(hh))
}

func registerSetLoggerLevel(ll *logger.ZapLogger, cfg *config.Config, lc fx.Lifecycle) {
	_ = ll.SetLevel(cfg.LCfg)
	lc.Append(utils.ToHook(ll))
}
