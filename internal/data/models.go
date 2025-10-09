package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/lib/pq"
	"github.com/yanyiwu/gojieba"
)

// ErrRecordNotFound will be returned when a record is not found in the database.
var (
	ErrRecordNotFound = errors.New("record not found")
	ErrEditConflict   = errors.New("edit conflict")
	ErrQuotaExceeded  = errors.New("quota exceeded")
)

type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (m Models) WithTx(ctx context.Context, opts *sql.TxOptions, fn func(*sql.Tx) error) error {
	tx, err := m.db.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()
	err = fn(tx)
	return err
}

func (m Models) WithTxRetry(
	ctx context.Context,
	opts *sql.TxOptions,
	maxRetries int,
	fn func(*sql.Tx) error,
) error {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := m.WithTx(ctx, opts, fn)
		if err == nil {
			return nil
		}
		var pqe *pq.Error
		switch {
		case errors.As(err, &pqe):
			m.logger.Info("Transaction retry", "attempt", attempt, "error", err, "pqcode", pqe.Code)
			time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
			continue
		case errors.Is(err, context.DeadlineExceeded):
			m.logger.Info(
				"Transaction retry due to context deadline exceeded",
				"attempt",
				attempt,
				"error",
				err,
			)
			time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
			continue
		}
		return err
	}

	return fmt.Errorf("transaction failed after %d retries", maxRetries)
}

type Models struct {
	Targets    TargetModel
	Actions    ActionModel
	Sessions   SessionModel
	Tokens     TokenModel
	Users      UserModel
	DailyQuota DailyQuotaModel
	db         *sql.DB
	logger     *slog.Logger
}

// NewModels returns a Models struct containing the initialized TargetModel.
func NewModels(db *sql.DB, jieba *gojieba.Jieba, logger *slog.Logger) Models {
	return Models{
		Targets:    TargetModel{DB: db, Jieba: jieba, logger: logger},
		Actions:    ActionModel{DB: db, Jieba: jieba, logger: logger},
		Sessions:   SessionModel{DB: db, Jieba: jieba},
		Tokens:     TokenModel{DB: db},
		Users:      UserModel{DB: db},
		DailyQuota: DailyQuotaModel{DB: db},

		db:     db,
		logger: logger,
	}
}

func (m Models) CreateTarget(target *Target, quota *DailyQuota, userUUID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.withQuotaTx(ctx, quota, userUUID, func(ctx context.Context, tx *sql.Tx) error {
		m.Targets.DB = tx
		m.logger.Info("Insert target")
		return m.Targets.Insert(ctx, target, userUUID)
	})
}

func (m Models) CreateAction(action *Action, quota *DailyQuota, userUUID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.withQuotaTx(ctx, quota, userUUID, func(ctx context.Context, tx *sql.Tx) error {
		m.Actions.DB = tx
		m.logger.Info("Insert action")
		return m.Actions.Insert(ctx, action, userUUID)
	})
}

func (m Models) CreateSession(session *Session, quota *DailyQuota, userUUID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.withQuotaTx(ctx, quota, userUUID, func(ctx context.Context, tx *sql.Tx) error {
		m.Sessions.DB = tx
		m.logger.Info("Insert session")
		return m.Sessions.Insert(ctx, session, userUUID)
	})
}

func (m Models) withQuotaTx(
	ctx context.Context,
	quota *DailyQuota,
	userUUID uuid.UUID,
	insert func(ctx context.Context, tx *sql.Tx) error,
) error {
	fn := func(tx *sql.Tx) error {
		m.DailyQuota.DB = tx

		limit := quota.Limit

		if err := m.DailyQuota.Insert(quota, userUUID); err != nil {
			return err
		}

		if err := m.DailyQuota.UpdateLock(quota, userUUID); err != nil {
			return err
		}
		if quota.Usage >= limit {
			return ErrQuotaExceeded
		}

		if err := insert(ctx, tx); err != nil {
			return err
		}

		if err := m.DailyQuota.Increment(quota, userUUID, 1); err != nil {
			return err
		}

		return nil
	}

	if err := m.WithTxRetry(ctx, nil, 3, fn); err != nil {
		return err
	}

	return nil
}
