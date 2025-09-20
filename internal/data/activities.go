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

type Activity struct {
	UUID          uuid.UUID    `json:"uuid"`
	CreatedAt     time.Time    `json:"created_at"`
	DueDate       sql.NullTime `json:"due_date,omitzero"`
	UpdatedAt     time.Time    `json:"updated_at"`
	LastActive    time.Time    `json:"last_active"`
	Title         string       `json:"title"`
	Description   string       `json:"description,omitzero"`
	Notes         string       `json:"notes,omitzero"`
	Version       int32        `json:"version"`
	Status        Status       `json:"status,omitzero"` // e.g., "queued", "in progress", "complete", "canceled"
	SerialID      int64        `json:"-"`               // Optional field for serial ID, not used in all contexts
	TargetUUID    uuid.UUID    `json:"target_uuid"`
	TargetTitle   string       `json:"target_title"`
	HasNotes      bool         `json:"has_notes"`
	SessionsCount int64        `json:"sessions_count"`
	Role          string       `json:"role"` // The user's role for this activity, e.g., "owner", "editor", "viewer"
}

func ValidateActivity(v *validator.Validator, activity *Activity) {
	v.Check(activity.TargetUUID != uuid.Nil, "target_uuid", "must be provided")
	v.Check(activity.Title != "", "title", "must be provided")
	v.Check(len(activity.Title) <= 200, "title", "must not be more than 200 characters long")
	v.Check(activity.Status != "", "status", "must be provided")
	v.Check(
		validator.PermittedValue(activity.Status, StatusSafelist...),
		"status",
		"must be one of 'queued', 'in progress', 'complete', 'canceled', or 'archived'",
	)
	if activity.DueDate.Valid {
		v.Check(
			activity.DueDate.Time.After(time.Now().AddDate(0, 0, -1)),
			"due_date",
			"must be in the future",
		)
	}
}

// ActivityModel struct type wraps a sql.DB connection pool and a Jieba instance.
type ActivityModel struct {
	DB    *sql.DB
	Jieba *gojieba.Jieba
}

func (m ActivityModel) Insert(activity *Activity, fts FTS, userUUID uuid.UUID) error {
	query := `
	WITH new_activity AS (
		INSERT INTO activities (target_uuid, title, description, notes, due_date, status)
		SELECT t.uuid, $2, $3, $4, $5, $6
        FROM targets t
	    WHERE t.uuid = $1 AND EXISTS (
			SELECT 1
			FROM acls a
			JOIN roles r ON a.role_code = r.code
			WHERE a.resource_type = 'target'
			AND a.resource_uuid = t.uuid
			AND a.user_uuid = $7
			AND r.rank <= (SELECT rank FROM roles WHERE code = 'editor')
		)
		RETURNING uuid, created_at, updated_at, version
	), grant_acl AS (
		INSERT INTO acls (user_uuid, resource_type, resource_uuid, role_code)
		SELECT $7, 'activity', uuid, 'owner' FROM new_activity
	), new_fts AS (
		INSERT INTO activities_fts (
			activity_uuid,
			fts_chinese_tsv,
			fts_english_tsv,
			fts_chinese_notes_tsv,
			fts_english_notes_tsv
		) SELECT
			uuid,
			setweight(to_tsvector('simple', $8), 'A') ||
			setweight(to_tsvector('simple', $9), 'B'),
			setweight(to_tsvector('english', $11), 'A') ||
			setweight(to_tsvector('english', $12), 'B'),
			to_tsvector('simple', $10),
			to_tsvector('english', $13)
		FROM new_activity
	)
	SELECT uuid, created_at, updated_at, version FROM new_activity;
	`
	// Consider adding index on acls_targets
	// CREATE INDEX ON acls_target (resource_uuid, user_uuid, role_code);

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{
		activity.TargetUUID,
		activity.Title,
		activity.Description,
		activity.Notes,
		activity.DueDate,
		activity.Status,
		userUUID,
		fts.TitleToken.Chinese,
		fts.DescriptionToken.Chinese,
		fts.NotesToken.Chinese,
		fts.TitleToken.English,
		fts.DescriptionToken.English,
		fts.NotesToken.English,
	}

	err := m.DB.QueryRowContext(ctx, query, args...).
		Scan(&activity.UUID, &activity.CreatedAt, &activity.UpdatedAt, &activity.Version)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrRecordNotFound
		default:
			return err
		}
	}

	return nil
}

func (m ActivityModel) Get(uuid, userUUID uuid.UUID, minRole string) (*Activity, error) {
	query := `
		WITH cutoff AS (
			SELECT rank AS cutoff
			FROM roles
			WHERE code = $3
		)
		SELECT 
			a.uuid,
			a.created_at,
			a.due_date,
			a.updated_at,
			a.last_active,
			a.title,
			a.description,
			a.notes,
			a.status,
			a.version,
			a.target_uuid,
			t.title
		FROM activities a 
		JOIN targets t ON a.target_uuid = t.uuid
		WHERE a.uuid = $1
			AND EXISTS (
				SELECT 1
				FROM acls ac
				JOIN roles r ON ac.role_code = r.code
				JOIN cutoff c ON r.rank <= c.cutoff
				WHERE ac.user_uuid = $2 AND (
					(ac.resource_type = 'activity' AND ac.resource_uuid = a.uuid)
					OR 
					(ac.resource_type = 'target' AND ac.resource_uuid = a.target_uuid)
				)
			);
		`
	args := []any{uuid, userUUID, minRole}

	var activity Activity

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(
		&activity.UUID,
		&activity.CreatedAt,
		&activity.DueDate,
		&activity.UpdatedAt,
		&activity.LastActive,
		&activity.Title,
		&activity.Description,
		&activity.Notes,
		&activity.Status,
		&activity.Version,
		&activity.TargetUUID,
		&activity.TargetTitle,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &activity, nil
}

func (m ActivityModel) Update(activity *Activity, fts FTS, userUUID uuid.UUID) error {
	// TODO: Need to perform permission tests if collaboration is ever introduced
	query := `
		WITH editor_cutoff AS (
			SELECT rank AS cutoff FROM roles WHERE code = 'editor'
		),
		owner_cutoff AS (
			SELECT rank AS cutoff FROM roles WHERE code = 'owner'
		),
		update_activity AS (
			UPDATE activities AS a 
			SET title = $1,
				description = $2,
				notes = $3,
				due_date = $4,
				status = $5,
				version = version + 1,
				updated_at = NOW(),
				last_active = NOW(),
				target_uuid = $8
			WHERE a.uuid = $6 AND a.version = $7 AND (
				(
					$8 IS NOT DISTINCT FROM a.target_uuid AND (
						EXISTS (
							SELECT 1
							FROM acls ac
							JOIN roles r ON ac.role_code = r.code
							JOIN editor_cutoff ec ON r.rank <= ec.cutoff
							WHERE ac.resource_type = 'activity'
							AND ac.resource_uuid = a.uuid
							AND ac.user_uuid = $9
						) 
						OR 
						EXISTS (
							SELECT 1
							FROM acls ac
							JOIN roles r ON ac.role_code = r.code
							JOIN editor_cutoff ec ON r.rank <= ec.cutoff
							WHERE ac.resource_type = 'target'
							AND ac.resource_uuid = a.target_uuid
							AND ac.user_uuid = $9
						)
					)
				)
				OR
				(
					$8 IS DISTINCT FROM a.target_uuid AND EXISTS (
						SELECT 1
						FROM acls ac
						JOIN roles r ON ac.role_code = r.code
						JOIN owner_cutoff oc ON r.rank <= oc.cutoff
						WHERE ac.resource_type = 'target'
							AND ac.resource_uuid = a.target_uuid
							AND ac.user_uuid = $9
					) AND EXISTS (
						SELECT 1
						FROM acls ac
						JOIN roles r ON ac.role_code = r.code
						JOIN owner_cutoff oc ON r.rank <= oc.cutoff
						WHERE ac.resource_type = 'target'
							AND ac.resource_uuid = $8
							AND ac.user_uuid = $9
					)
				)
			)
			RETURNING a.uuid, a.created_at, a.updated_at, a.last_active, a.version
		), update_fts AS (
			UPDATE activities_fts AS fts
			SET fts_chinese_tsv = setweight(to_tsvector('simple', $10), 'A') ||
					setweight(to_tsvector('simple', $11), 'B'),
				fts_english_tsv = setweight(to_tsvector('english', $13), 'A') ||
					setweight(to_tsvector('english', $14), 'B'),
				fts_chinese_notes_tsv = to_tsvector('simple', $12),
				fts_english_notes_tsv = to_tsvector('english', $15)
			FROM update_activity ua
			WHERE fts.activity_uuid = ua.uuid
		)
		SELECT a.created_at, a.updated_at, a.last_active, a.version FROM update_activity a;
	`

	args := []any{
		activity.Title,
		activity.Description,
		activity.Notes,
		activity.DueDate,
		activity.Status,
		activity.UUID,
		activity.Version,
		activity.TargetUUID,
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

	err := m.DB.QueryRowContext(ctx, query, args...).
		Scan(&activity.CreatedAt, &activity.UpdatedAt, &activity.LastActive, &activity.Version)
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

func (m ActivityModel) Delete(uuid, userUUID uuid.UUID) error {
	query := `
		WITH cutoff AS (
			SELECT rank AS cutoff
			FROM roles	
			WHERE code = 'owner'
		)
		DELETE FROM activities AS a USING cutoff AS c
		WHERE a.uuid = $1 AND (
			EXISTS (
				SELECT 1
				FROM acls ac
				JOIN roles r ON ac.role_code = r.code
				WHERE ac.resource_type = 'activity'
				AND ac.resource_uuid = a.uuid
				AND ac.user_uuid = $2
				AND r.rank <= c.cutoff
			)
			OR
			EXISTS (
				SELECT 1
				FROM acls ac
				JOIN roles r ON ac.role_code = r.code
				WHERE ac.resource_type = 'target'
				AND ac.resource_uuid = a.target_uuid
				AND ac.user_uuid = $2
				AND r.rank <= c.cutoff
			)
		)`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := m.DB.ExecContext(ctx, query, uuid, userUUID)
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

func (m ActivityModel) GetAll(
	token tokenizer.Tokenizer,
	filters Filters,
	targetUUID uuid.NullUUID,
	userUUID uuid.UUID,
) ([]*Activity, Metadata, error) {
	query := fmt.Sprintf(`
		WITH filtered AS MATERIALIZED (
			SELECT a.uuid, a.target_uuid
			FROM activities a
			JOIN activities_fts fts ON fts.activity_uuid = a.uuid
			JOIN targets t ON a.target_uuid = t.uuid
			WHERE ($1 = '' OR fts.fts_chinese_tsv @@ plainto_tsquery('simple', $1))
				AND ($2 = '' OR fts.fts_english_tsv @@ plainto_tsquery('english', $2))
				AND (CASE WHEN $3 = '' THEN TRUE ELSE a.status = $3::statuses END)
				AND ($4::uuid IS NULL OR a.target_uuid = $4::uuid)
				AND EXISTS (
					SELECT 1
					FROM acls ac
					JOIN roles r ON ac.role_code = r.code
					WHERE ac.user_uuid = $5
					AND r.rank <= (SELECT rank FROM roles WHERE code = 'viewer')
					AND (ac.resource_type, ac.resource_uuid) IN (
						('activity', a.uuid),
						('target', t.uuid)
					)
				)
		),
		total AS (
			SELECT count(*) AS total_count FROM filtered
		),
		paged AS (
			SELECT
				a.uuid,
				a.created_at,
				a.due_date,
				a.updated_at,
				a.last_active,
				a.title,
				a.description,
				a.status,
				a.version,
				a.serial_id,
				a.target_uuid,
				t.title as target_title,
				COALESCE(ss.sessions_count, 0) AS sessions_count,
				(btrim(COALESCE(a.notes, '')) <> '') AS has_notes,
				(CASE WHEN $1 <> '' THEN
					ts_rank(fts.fts_chinese_tsv, plainto_tsquery('simple', $1))
				ELSE 0 END) + (CASE WHEN $2 <> '' THEN
					ts_rank(fts.fts_english_tsv, plainto_tsquery('english', $2))
				ELSE 0 END) AS rank
			FROM filtered f
			JOIN activities a ON f.uuid = a.uuid
			JOIN activities_fts fts ON fts.activity_uuid = a.uuid
			JOIN targets t ON a.target_uuid = t.uuid
			LEFT JOIN (
				SELECT s.activity_uuid, COUNT(*) AS sessions_count
				FROM sessions s
				JOIN filtered fl ON fl.uuid = s.activity_uuid
				GROUP BY s.activity_uuid
			) ss ON ss.activity_uuid = a.uuid
			ORDER BY a.%s %s, rank DESC, a.serial_id DESC
			LIMIT $6 OFFSET $7
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
			p.target_uuid,
			p.target_title,
			p.sessions_count,
			p.has_notes,
			ur.role_code,
			p.rank
		FROM paged p
		JOIN LATERAL (
			SELECT ac.role_code
			FROM acls ac
			JOIN roles r ON ac.role_code = r.code
			WHERE ac.user_uuid = $5
				AND (
					(ac.resource_type = 'activity' AND ac.resource_uuid = p.uuid) OR
					(ac.resource_type = 'target' AND ac.resource_uuid = p.target_uuid)
				)
			ORDER BY r.rank ASC
			LIMIT 1
		) AS ur ON TRUE
		CROSS JOIN total
		ORDER BY p.%s %s, p.rank DESC, p.serial_id DESC
	`, filters.sortColumn(), filters.sortDirection(), filters.sortColumn(), filters.sortDirection())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{
		token.Chinese,
		token.English,
		filters.Status,
		targetUUID,
		userUUID,
		filters.limit(),
		filters.offset(),
	}

	rows, err := m.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Metadata{}, err
	}
	defer rows.Close()

	totalRecords := 0
	activities := []*Activity{}
	for rows.Next() {
		var activity Activity
		var ignored float64

		err := rows.Scan(
			&totalRecords,
			&activity.UUID,
			&activity.CreatedAt,
			&activity.DueDate,
			&activity.UpdatedAt,
			&activity.LastActive,
			&activity.Title,
			&activity.Description,
			&activity.Status,
			&activity.Version,
			&activity.SerialID,
			&activity.TargetUUID,
			&activity.TargetTitle,
			&activity.SessionsCount,
			&activity.HasNotes,
			&activity.Role,
			&ignored,
		)
		if err != nil {
			return nil, Metadata{}, err
		}

		activities = append(activities, &activity)
	}

	if err = rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	metadata := calculateMetadata(totalRecords, filters.Page, filters.PageSize)

	return activities, metadata, nil
}
