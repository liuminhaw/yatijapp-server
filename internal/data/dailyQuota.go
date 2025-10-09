package data

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofrs/uuid/v5"
)

type DailyQuota struct {
	UsageDate time.Time
	Resource  string
	Usage     int
	Limit     int
}

type DailyQuotaModel struct {
	DB     DBTX
	logger *slog.Logger
}

func (m DailyQuotaModel) Insert(quota *DailyQuota, userUUID uuid.UUID) error {
	query := `
		INSERT INTO daily_quota (user_id, usage_date, resource)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, usage_date, resource) DO NOTHING
	`

	args := []any{userUUID, quota.UsageDate, quota.Resource}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := m.DB.ExecContext(ctx, query, args...)

	return err
}

func (m DailyQuotaModel) UpdateLock(quota *DailyQuota, userUUID uuid.UUID) error {
	query := `
		SELECT quota_used
		FROM daily_quota
		WHERE user_id = $1 AND usage_date = $2 AND resource = $3
		FOR UPDATE
	`
	args := []any{userUUID, quota.UsageDate, quota.Resource}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.DB.QueryRowContext(ctx, query, args...).Scan(&quota.Usage)
}

func (m DailyQuotaModel) Increment(quota *DailyQuota, userUUID uuid.UUID, increment int) error {
	query := `
		UPDATE daily_quota
		SET quota_used = quota_used + $1
		WHERE user_id = $2 AND usage_date = $3 AND resource = $4
	`
	args := []any{increment, userUUID, quota.UsageDate, quota.Resource}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := m.DB.ExecContext(ctx, query, args...)
	return err
}
