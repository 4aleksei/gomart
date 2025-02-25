package store

import (
	"context"
	"errors"
	"time"
)

var ErrConflict = errors.New("data conflict")

type Store interface {
	AddUser(context.Context, User) (User, error)
	GetUser(context.Context, User) (User, error)

	InsertOrder(context.Context, Order) error
	InsertWithdraw(context.Context, Withdraw) error
	GetOrders(context.Context, uint64) ([]Order, error)
	GetOneOrder(context.Context, uint64) (Order, error)
	GetBalance(context.Context, uint64) (Balance, error)
	GetWithdrawals(context.Context, uint64) ([]Withdraw, error)

	GetOrdersForProcessing(context.Context) ([]Order, error)
	UpdateOrdersBalancesBatch(context.Context, []Order) error

	Close(context.Context)
	Ping(context.Context) error
}

type (
	User struct {
		Name     string `db:"name"`
		Password string `db:"password"`
		ID       uint64 `db:"id"`
	}

	Order struct {
		OrderID uint64    `db:"order_id"`
		UserID  uint64    `db:"user_id"`
		Status  string    `db:"status"`
		Accrual float64   `db:"accrual"`
		TimeU   time.Time `db:"uploaded_at"`
		TimeC   time.Time `db:"changed_at"`
	}

	Balance struct {
		UserID    uint64    `db:"user_id"`
		Accrual   float64   `db:"current"`
		Withdrawn float64   `db:"withdrawn"`
		TimeC     time.Time `db:"changed_at"`
	}

	Withdraw struct {
		UserID  uint64    `db:"user_id"`
		OrderID uint64    `db:"order_id"`
		Sum     float64   `db:"sum"`
		TimeC   time.Time `db:"processed_at"`
	}
)
