package service

import (
	"context"

	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/4aleksei/gmart/internal/common/models"
	"github.com/4aleksei/gmart/internal/common/store"
	"github.com/4aleksei/gmart/internal/common/store/pg"
	"github.com/4aleksei/gmart/internal/common/utils"
	"github.com/4aleksei/gmart/internal/gophermart/config"

	"github.com/4aleksei/gmart/internal/common/httpclientpool"
	"github.com/4aleksei/gmart/internal/common/httpclientpool/job"
)

type ServiceStore interface {
	AddUser(context.Context, store.User) (store.User, error)
	GetUser(context.Context, store.User) (store.User, error)
	GetBalance(context.Context, uint64) (store.Balance, error)
	InsertOrder(context.Context, store.Order) error
	InsertWithdraw(context.Context, store.Withdraw) error
	GetOrders(context.Context, uint64) ([]store.Order, error)

	GetWithdrawals(context.Context, uint64) ([]store.Withdraw, error)

	GetOneOrder(context.Context, uint64) (store.Order, error)

	GetOrdersForProcessing(context.Context) ([]store.Order, error)
	UpdateOrdersBalancesBatch(context.Context, []store.Order) error
}

type HandleService struct {
	store  ServiceStore
	key    string
	keySig string
	httpc  *httpclientpool.PoolHandler
	jid    job.JobID
}

var (
	ErrAuthenticationFailed = errors.New("authentication_failed")

	ErrBadPass = errors.New("name or password empty")

	ErrBadTypeValue = errors.New("invalid typeValue")
	ErrBadValue     = errors.New("error value conversion")
	ErrBadKindType  = errors.New("error kind type")

	ErrBadValueUser = errors.New("parse user_id number error")

	ErrOrderAlreadyLoaded = errors.New("order Already Loaded")

	ErrOrderAlreadyLoadedOtherUser = errors.New("error order already loaded other")

	ErrBalanceNotEnough = errors.New("balance not enouth")
)

func NewService(s ServiceStore, cfg *config.Config, h *httpclientpool.PoolHandler) *HandleService {
	return &HandleService{
		key:    cfg.Key,
		keySig: cfg.KeySignature,
		store:  s,
		httpc:  h,
	}
}

func (s *HandleService) RegisterUser(ctx context.Context, user models.UserRegistration) (string, error) {
	if user.Name == "" || user.Password == "" {
		return "", ErrBadPass
	}

	pass := utils.HashPass([]byte(user.Password), s.keySig) // try hash password

	userAdded, err := s.store.AddUser(ctx, store.User{Name: user.Name, Password: hex.EncodeToString(pass)})

	if err != nil {
		if errors.Is(err, pg.ErrAlreadyExists) {
			return "", ErrAuthenticationFailed
		}
		return "", err
	}

	id := strconv.FormatUint(userAdded.ID, 10)
	return id, nil
}

func (s *HandleService) LoginUser(ctx context.Context, user models.UserRegistration) (string, error) {
	if user.Name == "" || user.Password == "" {
		return "", ErrBadPass
	}

	userGet, err := s.store.GetUser(ctx, store.User{Name: user.Name})
	if err != nil {
		if errors.Is(err, pg.ErrRowNotFound) {
			return "", ErrAuthenticationFailed
		}
		return "", err
	}

	pass := utils.HashPass([]byte(user.Password), s.keySig) // try hash password
	if hex.EncodeToString(pass) != userGet.Password {
		return "", ErrAuthenticationFailed
	}

	id := strconv.FormatUint(userGet.ID, 10)
	return id, nil
}

func (s *HandleService) PostWithdraw(ctx context.Context, userIDStr string, withdraw models.Withdraw) error {
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("failed %w : %w", ErrBadValueUser, err)
	}

	orderID, err := strconv.ParseUint(withdraw.OrderID, 10, 64)
	if err != nil {
		return fmt.Errorf("failed %w : %w", ErrBadValue, err)
	}
	if !utils.ValidLuhn(orderID) {
		return fmt.Errorf("withdraw order failed Luhn %w ", ErrBadValue)
	}

	err = s.store.InsertWithdraw(ctx, store.Withdraw{OrderID: orderID, UserID: userID, Sum: withdraw.Sum})
	if err != nil {
		if errors.Is(err, pg.ErrBalanceNotEnough) {
			return ErrBalanceNotEnough
		}
		return err
	}
	return nil
}

func (s *HandleService) RegisterOrder(ctx context.Context, userIDStr, orderIDStr string) error {
	orderID, err := strconv.ParseUint(orderIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("failed %w : %w", ErrBadValue, err)
	}
	if !utils.ValidLuhn(orderID) {
		return fmt.Errorf("register order failed Luhn %w", ErrBadValue)
	}

	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("failed %w : %w", ErrBadValueUser, err)
	}

	err = s.store.InsertOrder(ctx, store.Order{OrderID: orderID, UserID: userID, Status: "NEW", Accrual: 0})
	if err != nil {
		if errors.Is(err, pg.ErrAlreadyExists) {
			one, err := s.store.GetOneOrder(ctx, orderID)
			if err != nil {
				return err
			}
			if one.UserID != userID {
				return ErrOrderAlreadyLoadedOtherUser
			}
			return ErrOrderAlreadyLoaded
		}
		return err
	}
	return nil
}

func (s *HandleService) GetOrders(ctx context.Context, userIDStr string) ([]models.Order, error) {
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed %w : %w", ErrBadValue, err)
	}
	vals, err := s.store.GetOrders(ctx, userID)
	if err != nil {
		return nil, err
	}
	valsret := make([]models.Order, len(vals))
	for i, v := range vals {
		valsret[i] = models.Order{OrderID: strconv.FormatUint(v.OrderID, 10), Status: v.Status, Accrual: v.Accrual, Time: v.TimeU}
	}
	return valsret, nil
}

func (s *HandleService) GetWithdrawals(ctx context.Context, userIDStr string) ([]models.Withdraw, error) {
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed %w : %w", ErrBadValue, err)
	}
	vals, err := s.store.GetWithdrawals(ctx, userID)
	if err != nil {
		return nil, err
	}
	valsret := make([]models.Withdraw, len(vals))
	for i, v := range vals {
		valsret[i] = models.Withdraw{OrderID: strconv.FormatUint(v.OrderID, 10), Sum: v.Sum, TimeC: v.TimeC}
	}
	return valsret, nil
}

func (s *HandleService) GetBalance(ctx context.Context, userIDStr string) (models.Balance, error) {
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	var valRet models.Balance
	if err != nil {
		return valRet, fmt.Errorf("failed %w : %w", ErrBadValue, err)
	}
	val, err := s.store.GetBalance(ctx, userID)
	if err != nil {
		if errors.Is(err, pg.ErrRowNotFound) {
			return valRet, nil
		}
		return valRet, err
	}
	valRet.Accrual = val.Accrual
	valRet.Withdrawn = val.Withdrawn
	return valRet, err
}

// Accrual Services

func (s *HandleService) GetOrdersForProcess(ctx context.Context) ([]store.Order, error) {
	vals, err := s.store.GetOrdersForProcessing(ctx)
	if err != nil {
		return nil, err
	}
	return vals, nil
}

func (s *HandleService) UpdateOrdersAndBalances(ctx context.Context, updOrders []store.Order) error {
	err := s.store.UpdateOrdersBalancesBatch(ctx, updOrders)
	if err != nil {
		return err
	}
	return nil
}

func (s *HandleService) newJid() job.JobID {
	s.jid++
	return s.jid
}

func (s *HandleService) sendRun(ctx context.Context, jobs chan job.Job, orders []store.Order) {
	defer close(jobs)

	for _, val := range orders {
		select {
		case <-ctx.Done():
			return
		default:
			id := s.newJid()
			jobs <- job.Job{ID: id, Value: val}
		}
	}
}

func (s *HandleService) SendOrdersToAccrual(ctx context.Context, orders []store.Order) (a map[uint64]store.Order, b int, c error) {
	wg := &sync.WaitGroup{}
	jobs := make(chan job.Job, s.httpc.WorkerCount*2)
	results := make(chan job.Result, s.httpc.WorkerCount*2)

	go s.sendRun(ctx, jobs, orders)

	s.httpc.StartPool(ctx, jobs, results, wg)

	go func() {
		wg.Wait()
		close(results)
	}()
	var waitSec int
	resOrders := make(map[uint64]store.Order)
	for res := range results {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		default:
			if res.Err == nil {
				if res.Result == httpclientpool.HTTPSuccessCode {
					resOrders[res.Value.OrderID] = res.Value
				} else if res.Result == httpclientpool.HTTPRetryCode {
					if res.WaitSec > waitSec {
						waitSec = res.WaitSec
					}
				}
			}
		}
	}
	return resOrders, waitSec, nil
}
