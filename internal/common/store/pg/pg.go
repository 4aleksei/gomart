package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/4aleksei/gmart/internal/common/logger"
	"github.com/4aleksei/gmart/internal/common/store"
	"github.com/4aleksei/gmart/internal/common/utils"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

type (
	PgStore struct {
		pool        *pgxpool.Pool
		DatabaseURI string
		limitbatch  int
		l           *logger.ZapLogger
	}
)

var (
	ErrAlreadyExists    = errors.New("already exists")
	ErrRowNotFound      = errors.New("not found")
	ErrBalanceNotEnough = errors.New("balance not enough")
)

func New(l *logger.ZapLogger) *PgStore {
	return &PgStore{l: l}
}

func ProbePGConnection(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgerrcode.IsConnectionException(pgErr.Code)
	}
	return false
}

func ProbePGDublicate(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgerrcode.IsIntegrityConstraintViolation(pgErr.Code) && pgErr.Code == "23505"
	}
	return false
}

func ProbePGErrorConstrain(err error) bool {
	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		fmt.Println(err)
		return pgerrcode.IsIntegrityConstraintViolation(pgErr.Code) && pgErr.Code == "23514"
	}
	return false
}

func ProbePGNoRows(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgerrcode.IsNoData(pgErr.Code)
	}
	return false
}

func (s *PgStore) Start(ctx context.Context) error {
	ctxB, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	err := utils.RetryAction(ctxB, utils.RetryTimes(), func(ctx context.Context) error {
		var err error
		s.pool, err = pgxpool.New(ctx, s.DatabaseURI)
		return err
	})

	if err != nil {
		return err
	}

	ctxTimeOutPing, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	err = utils.RetryAction(ctxTimeOutPing, utils.RetryTimes(), func(ctx context.Context) error {
		ctxTime, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		return s.pool.Ping(ctxTime)
	}, ProbePGConnection)

	if err != nil {
		return err
	}
	return nil
}

func (s *PgStore) Stop(ctx context.Context) error {
	s.Close(context.Background())
	return nil
}

const (
	queryDefault = `INSERT INTO users (name, password , created_at) VALUES ($1,$2,now())
	 RETURNING name, password, user_id`

	queryROrderDefault = `INSERT INTO orders (order_id, user_id , status ,accrual , uploaded_at, changed_at)
	       VALUES ($1,$2, $3 , $4 ,now(),now())
		    RETURNING order_id, user_id , status ,accrual`

	selectDefault = `SELECT name, password, user_id  FROM users WHERE name = $1`

	selectOrdersDefault = `SELECT  order_id, user_id , status ,accrual , uploaded_at, changed_at  FROM orders
	                        WHERE user_id = $1 ORDER BY uploaded_at DESC`

	selectOrdersProcessingDefault = `SELECT  order_id, user_id , status ,accrual , uploaded_at, changed_at  FROM orders
	                                WHERE status='NEW' or status='PROCESSING' or status='REGISTERED' ORDER BY uploaded_at DESC`

	selectOneOrderDefault = `SELECT  order_id, user_id , status ,accrual , uploaded_at, changed_at  FROM orders
	                                WHERE order_id = $1`

	selectBalanceDefault = `SELECT user_id , current ,withdrawn, changed_at  FROM balances WHERE user_id = $1`

	selectWithdrawalsDefault = `SELECT  user_id , order_id,  sum , processed_at FROM withdrawals
	                                 WHERE user_id = $1`

	queryInsertWithdrawDefault = `INSERT INTO withdrawals ( user_id, order_id ,sum , processed_at)
	       VALUES ($1,$2, $3 ,now()) RETURNING user_id, order_id ,sum , processed_at`

	queryBalanceDecDefault = `INSERT INTO balances (user_id , current ,withdrawn, changed_at) VALUES ($1,$2,$3,now())
		    ON CONFLICT (user_id) 
		DO UPDATE SET current=balances.current-excluded.current,
		              withdrawn=balances.withdrawn+excluded.withdrawn  , changed_at = now() 
		RETURNING user_id , current ,withdrawn, changed_at`

	queryBalanceIncDefault = `INSERT INTO balances (user_id , current ,withdrawn, changed_at) VALUES ($1,$2,$3,now())
		ON CONFLICT (user_id) 
	DO UPDATE SET current=balances.current+excluded.current , changed_at = now() 
	RETURNING user_id , current ,withdrawn, changed_at`

	queryCOrderDefault = `UPDATE orders SET status = $2  , accrual = $3 , changed_at = now() WHERE order_id = $1
			   RETURNING order_id, user_id , status ,accrual`
)

func (s *PgStore) InsertWithdraw(ctx context.Context, w store.Withdraw) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error begin tx: %w", err)
	}

	defer func() {
		defer func() { _ = tx.Rollback(ctx) }()
	}()

	row := tx.QueryRow(ctx, selectBalanceDefault, w.UserID)
	var o store.Balance
	if row != nil {
		_ = row.Scan(&o.UserID, &o.Accrual, &o.Withdrawn, &o.TimeC)
	}
	s.l.Logger.Debug("select ", zap.Any("balance", o))

	if o.Accrual < w.Sum {
		return ErrBalanceNotEnough
	}

	row = tx.QueryRow(ctx, queryBalanceDecDefault, w.UserID, w.Sum, w.Sum)
	if row != nil {
		var u store.Balance
		err := row.Scan(&u.UserID, &u.Accrual, &u.Withdrawn, &u.TimeC)
		if err != nil {
			if ProbePGDublicate(err) {
				return ErrAlreadyExists
			}
			return err
		}
		s.l.Logger.Debug("insert", zap.Any("new balance", u))
	} else {
		return ErrRowNotFound
	}

	rowW := tx.QueryRow(ctx, queryInsertWithdrawDefault, w.UserID, w.OrderID, w.Sum)
	if rowW != nil {
		var u store.Withdraw
		err := rowW.Scan(&u.UserID, &u.OrderID, &u.Sum, &u.TimeC)
		if err != nil {
			if ProbePGDublicate(err) {
				return ErrAlreadyExists
			}
			return err
		}
		s.l.Logger.Debug("insert ", zap.Any("withdraw", u))
	} else {
		return ErrRowNotFound
	}
	return tx.Commit(ctx)
}

func (s *PgStore) InsertOrder(ctx context.Context, o store.Order) error {
	row := s.pool.QueryRow(ctx, queryROrderDefault, o.OrderID, o.UserID, o.Status, o.Accrual)
	if row != nil {
		var u store.Order
		err := row.Scan(&u.OrderID, &u.UserID, &u.Status, &u.Accrual)
		if err != nil {
			if ProbePGDublicate(err) {
				return ErrAlreadyExists
			}
			return err
		}
	} else {
		return ErrRowNotFound
	}
	return nil
}

func (s *PgStore) GetOneOrder(ctx context.Context, id uint64) (store.Order, error) {
	row := s.pool.QueryRow(ctx, selectOneOrderDefault, id)
	var o store.Order
	if row != nil {
		err := row.Scan(&o.OrderID, &o.UserID, &o.Status, &o.Accrual, &o.TimeU, &o.TimeC)
		if err != nil {
			return o, err
		}
	} else {
		return o, ErrRowNotFound
	}
	return o, nil
}

func (s *PgStore) GetBalance(ctx context.Context, id uint64) (store.Balance, error) {
	row := s.pool.QueryRow(ctx, selectBalanceDefault, id)
	var o store.Balance
	if row != nil {
		err := row.Scan(&o.UserID, &o.Accrual, &o.Withdrawn, &o.TimeC)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return o, ErrRowNotFound
			}
			return o, err
		}
	} else {
		return o, ErrRowNotFound
	}
	return o, nil
}

const (
	defaultSliceCap int = 10
)

func (s *PgStore) GetWithdrawals(ctx context.Context, id uint64) ([]store.Withdraw, error) {
	rows, err := s.pool.Query(ctx, selectWithdrawalsDefault, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	withs := make([]store.Withdraw, 0, defaultSliceCap)
	for rows.Next() {
		var o store.Withdraw
		err := rows.Scan(&o.UserID, &o.OrderID, &o.Sum, &o.TimeC)
		if err != nil {
			return nil, err
		}
		withs = append(withs, o)
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}
	return withs, nil
}

func (s *PgStore) GetOrders(ctx context.Context, id uint64) ([]store.Order, error) {
	rows, err := s.pool.Query(ctx, selectOrdersDefault, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ores := make([]store.Order, 0, defaultSliceCap)
	for rows.Next() {
		var o store.Order
		err := rows.Scan(&o.OrderID, &o.UserID, &o.Status, &o.Accrual, &o.TimeU, &o.TimeC)
		if err != nil {
			return nil, err
		}
		ores = append(ores, o)
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}
	return ores, nil
}

func (s *PgStore) AddUser(ctx context.Context, u store.User) (store.User, error) {
	row := s.pool.QueryRow(ctx, queryDefault, u.Name, u.Password)
	if row != nil {
		err := row.Scan(&u.Name, &u.Password, &u.ID)
		if err != nil {
			if ProbePGDublicate(err) {
				return u, ErrAlreadyExists
			}
			return u, err
		}
	} else {
		return u, sql.ErrNoRows
	}
	return u, nil
}

func (s *PgStore) GetUser(ctx context.Context, u store.User) (store.User, error) {
	row := s.pool.QueryRow(ctx, selectDefault, u.Name)
	if row != nil {
		err := row.Scan(&u.Name, &u.Password, &u.ID)
		if err != nil {
			return u, err
		}
	} else {
		return u, ErrRowNotFound
	}
	return u, nil
}

func (s *PgStore) GetOrdersForProcessing(ctx context.Context) ([]store.Order, error) {
	rows, err := s.pool.Query(ctx, selectOrdersProcessingDefault)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ores := make([]store.Order, 0, defaultSliceCap)
	for rows.Next() {
		var o store.Order
		err := rows.Scan(&o.OrderID, &o.UserID, &o.Status, &o.Accrual, &o.TimeU, &o.TimeC)
		if err != nil {
			return nil, err
		}
		ores = append(ores, o)
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}
	return ores, nil
}

func (s *PgStore) UpdateOrdersBalancesBatch(ctx context.Context, orders []store.Order) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error begin tx: %w", err)
	}

	defer func() {
		defer func() { _ = tx.Rollback(ctx) }()
	}()

	var indexLimit int
	if s.limitbatch != 0 && len(orders) > s.limitbatch {
		indexLimit = s.limitbatch
	} else {
		indexLimit = len(orders)
	}

	for index := 0; index < len(orders); index += indexLimit {
		if (index + indexLimit) > len(orders) {
			indexLimit = len(orders) - index
		}

		batch := &pgx.Batch{}
		for i := 0; i < indexLimit; i++ {
			batch.Queue(queryCOrderDefault, orders[i+index].OrderID, orders[i+index].Status, orders[i+index].Accrual)
		}

		for i := 0; i < indexLimit; i++ {
			batch.Queue(queryBalanceIncDefault, orders[i+index].UserID, orders[i+index].Accrual, 0)
		}

		br := tx.SendBatch(ctx, batch)

		if e := br.Close(); e != nil {
			return fmt.Errorf("closing batch result: %w", e)
		}
	}
	return tx.Commit(ctx)
}

func (s *PgStore) Close(ctx context.Context) {
	s.pool.Close()
}

func (s *PgStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}
