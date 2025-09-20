package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/liuminhaw/sessions-of-life/internal/tokenizer"
	"github.com/liuminhaw/sessions-of-life/internal/validator"
	"github.com/yanyiwu/gojieba"
)

type Target struct {
	UUID            uuid.UUID    `json:"uuid"`
	CreatedAt       time.Time    `json:"created_at"`
	DueDate         sql.NullTime `json:"due_date,omitzero"`
	UpdatedAt       time.Time    `json:"updated_at"`
	LastActive      time.Time    `json:"last_active"`
	Title           string       `json:"title"`
	Description     string       `json:"description,omitzero"`
	Notes           string       `json:"notes,omitzero"`
	Version         int32        `json:"version"`
	Status          Status       `json:"status,omitzero"` // e.g., "queued", "in progress", "complete", "canceled"
	SerialID        int64        `json:"-"`               // Optional field for serial ID, not used in all contexts
	HasNotes        bool         `json:"has_notes"`
	ActivitiesCount int64        `json:"activities_count"`
	Role            string       `json:"role"` // The user's role for this target, e.g., "owner", "editor", "viewer"
}

func ValidateTarget(v *validator.Validator, target *Target) {
	v.Check(target.Title != "", "title", "must be provided")
	v.Check(len(target.Title) <= 200, "title", "must not be more than 200 characters long")
	v.Check(target.Status != "", "status", "must be provided")
	v.Check(
		validator.PermittedValue(target.Status, StatusSafelist...),
		"status",
		"must be one of 'queued', 'in progress', 'complete', 'canceled', or 'archived'",
	)
	if target.DueDate.Valid {
		v.Check(
			target.DueDate.Time.After(time.Now().AddDate(0, 0, -1)),
			"due_date",
			"must be in the future",
		)
	}
}

// TargetModel struct type wraps a sql.DB connection pool.
type TargetModel struct {
	DB    *sql.DB
	Jieba *gojieba.Jieba
}

func (t TargetModel) Insert(target *Target, fts FTS, userUUID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `
		WITH new_target AS (
			INSERT INTO targets (title, description, notes, due_date, status)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING uuid, created_at, updated_at, version
		), grant_acl AS (
			INSERT INTO acls (user_uuid, resource_type, resource_uuid, role_code)
			SELECT $6, 'target', uuid, 'owner' FROM new_target
		), new_fts AS (
			INSERT INTO targets_fts (
				target_uuid, 
				fts_chinese_tsv, 
				fts_english_tsv, 
				fts_chinese_notes_tsv, 
				fts_english_notes_tsv
			) SELECT
				uuid,
				setweight(to_tsvector('simple', $7), 'A') ||
				setweight(to_tsvector('simple', $8), 'B'),
				setweight(to_tsvector('english', $10), 'A') ||
				setweight(to_tsvector('english', $11), 'B'),
				to_tsvector('simple', $9),
				to_tsvector('english', $12)
			FROM new_target
		)
		SELECT uuid, created_at, updated_at, version FROM new_target;
	`
	args := []any{
		target.Title,
		target.Description,
		target.Notes,
		target.DueDate,
		target.Status,
		userUUID,
		fts.TitleToken.Chinese,
		fts.DescriptionToken.Chinese,
		fts.NotesToken.Chinese,
		fts.TitleToken.English,
		fts.DescriptionToken.English,
		fts.NotesToken.English,
	}

	err := t.DB.QueryRowContext(ctx, query, args...).
		Scan(&target.UUID, &target.CreatedAt, &target.UpdatedAt, &target.Version)
	if err != nil {
		return err
	}

	return nil
}

func (t TargetModel) Get(uuid, userUUID uuid.UUID, minRole string) (*Target, error) {
	query := `
		SELECT 
			t.uuid, 
			t.created_at, 
			t.due_date, 
			t.updated_at, 
			t.last_active,
			t.title, 
			t.description, 
			t.notes, 
			t.status, 
			t.version
		FROM targets t
		JOIN acls a ON a.resource_type = 'target' AND a.resource_uuid = t.uuid
		JOIN roles r ON a.role_code = r.code
		WHERE uuid = $1 
			AND a.user_uuid = $2 
			AND r.rank <= (SELECT rank FROM roles WHERE code = $3)
	`

	var target Target

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := t.DB.QueryRowContext(ctx, query, uuid, userUUID, minRole).Scan(
		&target.UUID,
		&target.CreatedAt,
		&target.DueDate,
		&target.UpdatedAt,
		&target.LastActive,
		&target.Title,
		&target.Description,
		&target.Notes,
		&target.Status,
		&target.Version,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &target, nil
}

func (t TargetModel) Update(target *Target, fts FTS, userUUID uuid.UUID) error {
	query := `
		WITH update_target AS(
			UPDATE targets AS t
			SET title = $1, 
				description = $2, 
				notes = $3, 
				due_date = $4, 
				status = $5, 
				version = version + 1, 
				updated_at = NOW(), 
				last_active = NOW()
			WHERE t.uuid = $6 AND t.version = $7 AND EXISTS (
				SELECT 1
				FROM acls a
				JOIN roles r ON a.role_code = r.code
				WHERE a.resource_type = 'target'
				AND a.resource_uuid = $6
				AND a.user_uuid = $8
				AND r.rank <= (SELECT rank FROM roles WHERE code = 'editor')
			)
			RETURNING t.uuid, t.created_at, t.updated_at, t.version
		), update_fts AS (
			UPDATE targets_fts AS fts
			SET fts_chinese_tsv = setweight(to_tsvector('simple', $9), 'A') ||
					setweight(to_tsvector('simple', $10), 'B'),
				fts_english_tsv = setweight(to_tsvector('english', $12), 'A') ||
					setweight(to_tsvector('english', $13), 'B'),
				fts_chinese_notes_tsv = to_tsvector('simple', $11),
				fts_english_notes_tsv = to_tsvector('english', $14)
			FROM update_target ut
			WHERE fts.target_uuid = ut.uuid
		)
		SELECT t.created_at, t.updated_at, t.version FROM update_target t;
	`

	args := []any{
		target.Title,
		target.Description,
		target.Notes,
		target.DueDate,
		target.Status,
		target.UUID,
		target.Version,
		userUUID,
		fts.TitleToken.Chinese,
		fts.DescriptionToken.Chinese,
		fts.NotesToken.Chinese,
		fts.TitleToken.English,
		fts.DescriptionToken.English,
		fts.NotesToken.English,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := t.DB.QueryRowContext(ctx, query, args...).
		Scan(&target.CreatedAt, &target.UpdatedAt, &target.Version)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}

	return nil
}

func (t TargetModel) Delete(uuid, userUUID uuid.UUID) error {
	query := `
		DELETE FROM targets
		WHERE uuid = $1 AND EXISTS (
			SELECT 1
			FROM acls a
			JOIN roles r ON a.role_code = r.code
			WHERE a.resource_type = 'target'	
			AND a.resource_uuid = $1
			AND a.user_uuid = $2
			AND r.rank <= (SELECT rank FROM roles WHERE code = 'owner')
		)
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := t.DB.ExecContext(ctx, query, uuid, userUUID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}

func (t TargetModel) GetAllForUser(
	token tokenizer.Tokenizer,
	filters Filters,
	userUUID uuid.UUID,
) ([]*Target, Metadata, error) {
	query := fmt.Sprintf(`
		WITH filtered AS MATERIALIZED (
			SELECT t.uuid
			FROM targets t
			JOIN targets_fts fts ON fts.target_uuid = t.uuid
			WHERE ($1 = '' OR fts.fts_chinese_tsv @@ plainto_tsquery('simple', $1))
				AND ($2 = '' OR fts.fts_english_tsv @@ plainto_tsquery('english', $2))
				AND ($3 = '' OR t.status = $3::statuses)
				AND EXISTS (
					SELECT 1
					FROM acls ac
					JOIN roles r ON ac.role_code = r.code	
					WHERE ac.user_uuid = $4
					AND ac.resource_type = 'target'
					AND ac.resource_uuid = t.uuid
					AND r.rank <= (SELECT rank FROM roles WHERE code = 'viewer')
				)
		),
		total AS (
			SELECT COUNT(*) AS total_count FROM filtered
		),
		paged AS (
			SELECT 
				t.uuid,
				t.created_at,
				t.due_date,
				t.updated_at,
				t.last_active,
				t.title,
				t.description,
				t.status,
				t.version,
				t.serial_id,
				COALESCE(ss.activities_count, 0) AS activities_count,
				(btrim(COALESCE(t.notes, '')) <> '') AS has_notes,
				(CASE WHEN $1 <> '' THEN
					ts_rank(fts.fts_chinese_tsv, plainto_tsquery('simple', $1))
				ELSE 0 END) + (CASE WHEN $2 <> '' THEN
					ts_rank(fts.fts_english_tsv, plainto_tsquery('english', $2))
				ELSE 0 END) AS rank
			FROM filtered f
			JOIN targets t ON f.uuid = t.uuid
			JOIN targets_fts fts ON fts.target_uuid = t.uuid
			LEFT JOIN (
				SELECT a.target_uuid, COUNT(*) AS activities_count
				FROM activities a
				JOIN filtered fl ON fl.uuid = a.target_uuid
				GROUP BY a.target_uuid
			) ss ON ss.target_uuid = t.uuid
			ORDER BY t.%s %s, rank DESC, t.serial_id DESC
			LIMIT $5 OFFSET $6
		)
		SELECT
			total.total_count,
			p.uuid,
			p.created_at,
			p.due_date,
			p.updated_at,
			p.last_active,
			p.title,
			p.description,
			p.status,
			p.version,
			p.serial_id,
			p.activities_count,
			p.has_notes,
			ac.role_code,
			p.rank
		FROM paged p
		JOIN acls ac
			ON ac.user_uuid = $4
			AND ac.resource_type = 'target'
			AND ac.resource_uuid = p.uuid
		CROSS JOIN total
		ORDER BY p.%s %s, p.rank DESC, p.serial_id DESC
	`, filters.sortColumn(), filters.sortDirection(), filters.sortColumn(), filters.sortDirection())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{
		token.Chinese,
		token.English,
		filters.Status,
		userUUID,
		filters.limit(),
		filters.offset(),
	}

	rows, err := t.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Metadata{}, err
	}
	defer rows.Close()

	totalRecords := 0
	targets := []*Target{}
	for rows.Next() {
		var target Target
		var ignored float64

		err := rows.Scan(
			&totalRecords,
			&target.UUID,
			&target.CreatedAt,
			&target.DueDate,
			&target.UpdatedAt,
			&target.LastActive,
			&target.Title,
			&target.Description,
			&target.Status,
			&target.Version,
			&target.SerialID,
			&target.ActivitiesCount,
			&target.HasNotes,
			&target.Role,
			&ignored,
		)
		if err != nil {
			return nil, Metadata{}, err
		}

		targets = append(targets, &target)
	}

	if err = rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	metadata := calculateMetadata(totalRecords, filters.Page, filters.PageSize)

	return targets, metadata, nil
}
