package accrual

import (
	"context"
	"sync"
	"time"

	"github.com/4aleksei/gmart/internal/common/logger"
	"github.com/4aleksei/gmart/internal/common/store"
	"github.com/4aleksei/gmart/internal/common/utils"
	"github.com/4aleksei/gmart/internal/gophermart/service"
	"go.uber.org/zap"

	"github.com/4aleksei/gmart/internal/gophermart/config"
)

type (
	HandlersAccrual struct {
		cfg    *config.Config
		l      *logger.ZapLogger
		s      *service.HandleService
		wg     sync.WaitGroup
		cancel context.CancelFunc
	}
)

func NewAccrual(cfg *config.Config, s *service.HandleService, l *logger.ZapLogger) *HandlersAccrual {
	return &HandlersAccrual{
		cfg: cfg,
		l:   l,
		s:   s,
	}
}

func (a *HandlersAccrual) Start(ctx context.Context) error {
	ctxCancel, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.wg.Add(1)
	go a.mainAccrual(ctxCancel)
	return nil
}

func (a *HandlersAccrual) Stop(ctx context.Context) error {
	a.cancel()
	a.wg.Wait()
	return nil
}

func (a *HandlersAccrual) mainAccrual(ctx context.Context) {
	defer a.wg.Done()

	a.l.Logger.Info("Start Accrual client.")
	var waitSec int64 = 0
	for {
		utils.SleepCancellable(ctx, time.Duration(a.cfg.PollInterval+waitSec)*time.Second)
		select {
		case <-ctx.Done():
			return
		default:
			waitSec = 0
			orders, err := a.s.GetOrdersForProcess(ctx)
			if err != nil {
				a.l.Logger.Debug("Accrual: error request new orders ", zap.Error(err))
				continue
			}

			a.l.Logger.Debug("Accrual: get new ORDERS", zap.Int("len ", len(orders)))

			if len(orders) == 0 {
				continue
			}

			resOrders, w, err := a.s.SendOrdersToAccrual(ctx, orders)
			if w > 0 {
				waitSec = int64(w)
			}
			if err != nil {
				a.l.Logger.Debug("Accrual: error send orders ", zap.Error(err))
				continue
			}

			a.l.Logger.Debug("Accrual: get resOrders", zap.Int("len ", len(resOrders)))
			updOrders := make([]store.Order, 0)
			for i := 0; i < len(orders); i++ {
				if val, ok := resOrders[orders[i].OrderID]; ok {
					if orders[i].Status != val.Status {
						a.l.Logger.Debug("update", zap.String("oldstatus", orders[i].Status), zap.Any("new status", val))
						updOrders = append(updOrders, val)
					}
				}
			}

			a.l.Logger.Debug("Accrual: do update orders", zap.Int("len ", len(updOrders)))

			if len(updOrders) == 0 {
				continue
			}

			err = a.s.UpdateOrdersAndBalances(ctx, updOrders)
			if err != nil {
				a.l.Logger.Debug("Accrual: error update orders and balances ", zap.Error(err))
			}
		}
	}
}
