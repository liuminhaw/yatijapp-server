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

type Session struct {
	UUID        string       `json:"uuid"`
	StartsAt    time.Time    `json:"starts_at"`
	EndsAt      sql.NullTime `json:"ends_at"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"last_active"`
	Notes       string       `json:"notes"`
	Version     int32        `json:"version"`
	ActionUUID  uuid.UUID    `json:"action_uuid"`
	ActionTitle string       `json:"action_title"`
	TargetUUID  uuid.UUID    `json:"target_uuid"`
	TargetTitle string       `json:"target_title"`
	HasNotes    bool         `json:"has_notes"`
	Role        string       `json:"role"` // The user's role for this session, e.g., "owner", "editor", "viewer"
}

func ValidateSession(v *validator.Validator, session *Session) {
	v.Check(session.ActionUUID != uuid.Nil, "action_uuid", "must be provided")
	if session.EndsAt.Valid {
		v.Check(session.EndsAt.Time.After(session.StartsAt), "ends_at", "must be after starts_at")
	}
}

type SessionModel struct {
	DB    *sql.DB
	Jieba *gojieba.Jieba
}

func (m SessionModel) Insert(session *Session, fts FTS, userUUID uuid.UUID) error {
	query := `
	WITH cutoff AS (
		SELECT rank AS cutoff
		FROM roles
		WHERE code = 'editor'
	),
	new_session AS (
		INSERT INTO sessions (action_uuid)
		SELECT a.uuid
		FROM actions a 
		WHERE a.uuid = $1 AND EXISTS (
			SELECT 1
			FROM acls ac
			JOIN roles r ON ac.role_code = r.code
			JOIN cutoff c ON r.rank <= c.cutoff
			WHERE ac.user_uuid = $2 AND (
				(ac.resource_type = 'action' AND ac.resource_uuid = a.uuid)
				OR
				(ac.resource_type = 'target' AND ac.resource_uuid = a.target_uuid)
			)
		)
		RETURNING uuid, starts_at, created_at, updated_at, version
	), grant_acl AS (
		INSERT INTO acls (user_uuid, resource_type, resource_uuid, role_code)
		SELECT $2, 'session', uuid, 'owner' FROM new_session
	), new_fts AS (
		INSERT INTO sessions_fts (session_uuid, fts_chinese_notes_tsv, fts_english_notes_tsv)
		SELECT uuid, to_tsvector('simple', $3), to_tsvector('english', $4)
		FROM new_session
	)
	SELECT uuid, starts_at, created_at, updated_at, version FROM new_session;
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{session.ActionUUID, userUUID, fts.NotesToken.Chinese, fts.NotesToken.English}

	err := m.DB.QueryRowContext(ctx, query, args...).
		Scan(
			&session.UUID,
			&session.StartsAt,
			&session.CreatedAt,
			&session.UpdatedAt,
			&session.Version,
		)
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

func (m SessionModel) Get(uuid, userUUID uuid.UUID, minRole string) (*Session, error) {
	query := `
		SELECT 
			s.uuid, 
			s.starts_at, 
			s.ends_at, 
			s.created_at,
			s.updated_at,
			s.notes, 
			s.version, 
			s.action_uuid, 
			a.title,
			a.target_uuid,
			t.title
		FROM sessions s
		JOIN actions a ON s.action_uuid = a.uuid
		JOIN targets t ON a.target_uuid = t.uuid
		WHERE s.uuid = $1 AND EXISTS (
			SELECT 1
			FROM acls ac
			JOIN roles r ON ac.role_code = r.code
			WHERE 
				ac.user_uuid = $2 AND 
				r.rank <= (SELECT rank FROM roles WHERE code = $3 LIMIT 1) 
				AND (ac.resource_type, ac.resource_uuid) IN (
					('session', s.uuid),
					('action', a.uuid),
					('target', t.uuid)
				)
		)
	`
	args := []any{uuid, userUUID, minRole}

	var session Session

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(
		&session.UUID,
		&session.StartsAt,
		&session.EndsAt,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.Notes,
		&session.Version,
		&session.ActionUUID,
		&session.ActionTitle,
		&session.TargetUUID,
		&session.TargetTitle,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &session, nil
}

func (m SessionModel) Update(session *Session, fts FTS, userUUID uuid.UUID) error {
	query := `
		WITH editor_cutoff AS (
			SELECT rank AS cutoff FROM roles WHERE code = 'editor'
		),
		owner_cutoff AS (
			SELECT rank AS cutoff FROM roles WHERE code = 'owner'
		), 
		update_session AS (
			UPDATE sessions AS s
			SET starts_at = $1,
				ends_at = $2,
				notes = $3,
				updated_at = NOW(),
				version = version + 1,
				action_uuid = $4
			WHERE s.uuid = $5 AND s.version = $6 AND (
				(
					$4 IS NOT DISTINCT FROM s.action_uuid AND (
						EXISTS (
							SELECT 1
							FROM acls ac
							JOIN roles r ON ac.role_code = r.code
							JOIN editor_cutoff ec ON r.rank <= ec.cutoff
							WHERE ac.resource_type = 'session'
							AND ac.resource_uuid = s.uuid
							AND ac.user_uuid = $7
						)
						OR
						EXISTS (
							SELECT 1
							FROM acls ac
							JOIN roles r ON ac.role_code = r.code
							JOIN editor_cutoff ec ON r.rank <= ec.cutoff
							WHERE ac.resource_type = 'action'
							AND ac.resource_uuid = s.action_uuid
							AND ac.user_uuid = $7
						)
						OR
						EXISTS (
							SELECT 1
							FROM acls ac
							JOIN roles r ON ac.role_code = r.code
							JOIN editor_cutoff ec ON r.rank <= ec.cutoff
							JOIN actions a ON s.action_uuid = a.uuid
							WHERE ac.resource_type = 'target'
							AND ac.resource_uuid = a.target_uuid
							AND ac.user_uuid = $7
						)
					)
				)
				OR
				(
					$4 IS DISTINCT FROM s.action_uuid AND EXISTS (
						SELECT 1
						FROM acls ac
						JOIN roles r ON ac.role_code = r.code
						JOIN owner_cutoff oc ON r.rank <= oc.cutoff
						WHERE ac.resource_type = 'action'
							AND ac.resource_uuid = s.action_uuid
							AND ac.user_uuid = $7
					) AND EXISTS (
						SELECT 1
						FROM acls ac
						JOIN roles r ON ac.role_code = r.code
						JOIN owner_cutoff oc ON r.rank <= oc.cutoff
						WHERE ac.resource_type = 'action'
							AND ac.resource_uuid = $4
							AND ac.user_uuid = $7
					)
				)
			)
			RETURNING uuid, created_at, updated_at, version
		), update_fts AS (
			UPDATE sessions_fts AS fts
			SET fts_chinese_notes_tsv = to_tsvector('simple', $8),
				fts_english_notes_tsv = to_tsvector('english', $9)
			FROM update_session us
			WHERE fts.session_uuid = us.uuid
		)
		SELECT created_at, updated_at, version FROM update_session;
	`

	args := []any{
		session.StartsAt,
		session.EndsAt,
		session.Notes,
		session.ActionUUID,
		session.UUID,
		session.Version,
		userUUID,
		fts.NotesToken.Chinese,
		fts.NotesToken.English,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).
		Scan(&session.CreatedAt, &session.UpdatedAt, &session.Version)
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

func (m SessionModel) Delete(uuid, userUUID uuid.UUID) error {
	query := `
		WITH cutoff AS (
			SELECT rank AS cutoff FROM roles WHERE code = 'owner'
		)
		DELETE FROM sessions s
		WHERE s.uuid = $1 AND EXISTS(
			SELECT 1
			FROM acls ac
			JOIN roles r ON ac.role_code = r.code
			JOIN cutoff c ON r.rank <= c.cutoff
			JOIN actions a ON s.action_uuid = a.uuid
			WHERE ac.user_uuid = $2
				AND (ac.resource_type, ac.resource_uuid) IN (
					('session', s.uuid),
					('action', s.action_uuid),
					('target', a.target_uuid)
				)
		);
	`

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

func (m SessionModel) GetAll(
	token tokenizer.Tokenizer,
	filters Filters,
	actionUUID uuid.NullUUID,
	userUUID uuid.UUID,
) ([]*Session, Metadata, error) {
	query := fmt.Sprintf(`
		WITH filtered AS MATERIALIZED (
			SELECT s.uuid, s.action_uuid, a.target_uuid
			FROM sessions s
			JOIN sessions_fts fts ON fts.session_uuid = s.uuid
			JOIN actions a ON s.action_uuid = a.uuid
			JOIN targets t ON a.target_uuid = t.uuid
			WHERE ($1 = '' OR fts.fts_chinese_notes_tsv @@ plainto_tsquery('simple', $1))
				AND ($2 = '' OR fts.fts_english_notes_tsv @@ plainto_tsquery('english', $2))
				AND ($3::uuid IS NULL OR s.action_uuid = $3)
				AND EXISTS (
					SELECT 1
					FROM acls ac
					JOIN roles r ON ac.role_code = r.code
					WHERE ac.user_uuid = $4
					AND r.rank <= (SELECT rank FROM roles WHERE code = 'viewer')
					AND (ac.resource_type, ac.resource_uuid) IN (
						('session', s.uuid),
						('action', a.uuid),
						('target', t.uuid)
					)
				)
		),
		total AS (
			SELECT COUNT(*) AS total_count FROM filtered
		),
		paged AS (
			SELECT 
				s.uuid,
				s.starts_at,
				s.ends_at,
				s.created_at,
				s.updated_at,
				s.version,
				s.action_uuid,
				a.title AS action_title,
				a.target_uuid,
				t.title AS target_title,
				(btrim(COALESCE(s.notes, '')) <> '') AS has_notes,
				(CASE WHEN $1 <> '' THEN 
					ts_rank(fts.fts_chinese_notes_tsv, plainto_tsquery('simple', $1)) 
				ELSE 0 END) + (CASE WHEN $2 <> '' THEN 
					ts_rank(fts.fts_english_notes_tsv, plainto_tsquery('english', $2)) 
				ELSE 0 END) AS rank
			FROM filtered f
			JOIN sessions s ON f.uuid = s.uuid
			JOIN sessions_fts fts ON s.uuid = fts.session_uuid
			JOIN actions a ON s.action_uuid = a.uuid
			JOIN targets t ON a.target_uuid = t.uuid
			ORDER BY s.%s %s, rank DESC, s.uuid DESC
			LIMIT $5 OFFSET $6
		)
		SELECT 
			total.total_count,
			p.uuid,
			p.starts_at,
			p.ends_at,
			p.created_at,
			p.updated_at,
			p.version,
			p.action_uuid,
			p.action_title,
			p.target_uuid,
			p.target_title,
			p.has_notes,
			ur.role_code,
		    p.rank
		FROM paged p
		JOIN LATERAL (
			SELECT ac.role_code
			FROM acls ac
			JOIN roles r ON ac.role_code = r.code
			WHERE ac.user_uuid = $4
				AND (
					(ac.resource_type = 'session' AND ac.resource_uuid = p.uuid) OR
					(ac.resource_type = 'action' AND ac.resource_uuid = p.action_uuid) OR
					(ac.resource_type = 'target' AND ac.resource_uuid = p.target_uuid)
				)
			ORDER BY r.rank ASC
			LIMIT 1
		) AS ur ON TRUE
		CROSS JOIN total
		ORDER BY p.%s %s, p.rank DESC, p.uuid DESC;
		`, filters.sortColumn(), filters.sortDirection(), filters.sortColumn(), filters.sortDirection())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{
		token.Chinese,
		token.English,
		actionUUID,
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
	sessions := []*Session{}
	for rows.Next() {
		var session Session
		var ignored float64

		err := rows.Scan(
			&totalRecords,
			&session.UUID,
			&session.StartsAt,
			&session.EndsAt,
			&session.CreatedAt,
			&session.UpdatedAt,
			&session.Version,
			&session.ActionUUID,
			&session.ActionTitle,
			&session.TargetUUID,
			&session.TargetTitle,
			&session.HasNotes,
			&session.Role,
			&ignored,
		)
		if err != nil {
			return nil, Metadata{}, err
		}

		sessions = append(sessions, &session)
	}

	if err = rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	metadata := calculateMetadata(totalRecords, filters.Page, filters.PageSize)

	return sessions, metadata, nil
}
