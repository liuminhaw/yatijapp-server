package data

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/lib/pq"
	"github.com/liuminhaw/yatijapp/internal/tokenizer"
	"github.com/yanyiwu/gojieba"
)

type Record struct {
	Kind        string    `json:"kind"`
	UUID        uuid.UUID `json:"uuid"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      Status    `json:"status"`
	HasNotes    bool      `json:"has_notes"`
	LastActive  time.Time `json:"last_active"`
}

type RecordModel struct {
	DB     DBTX
	Jieba  *gojieba.Jieba
	logger *slog.Logger
}

func (m RecordModel) GetAll(
	token tokenizer.Tokenizer,
	filters Filters,
	userUUID uuid.UUID,
) ([]*Record, Metadata, error) {
	query := `
		WITH search AS (
			SELECT $1::text AS chinese, $2::text AS english
		),
		viewer_cutoff AS (
			SELECT rank AS cutoff FROM roles WHERE code = 'viewer'
		),
		hits AS (
			SELECT 'target'::text AS kind,
					0 as kind_order,
					t.uuid,
					t.title,
					t.description,
					t.status AS status,
					(btrim(COALESCE(t.notes, '')) <> '') AS has_notes,
					t.last_active,
					(COALESCE(ts_rank(tf.fts_chinese_tsv, plainto_tsquery('simple', search.chinese)), 0) +
					COALESCE(ts_rank(tf.fts_english_tsv, plainto_tsquery('english', search.english)), 0)) AS score
			FROM search
			JOIN targets t ON TRUE
			JOIN targets_fts tf ON tf.target_uuid = t.uuid
			JOIN viewer_cutoff vc ON TRUE
			WHERE (search.chinese = '' OR tf.fts_chinese_tsv @@ plainto_tsquery('simple', search.chinese))
			  AND (search.english = '' OR tf.fts_english_tsv @@ plainto_tsquery('english', search.english))
			  AND ($6 = '{}'::statuses[] OR t.status = ANY($6::statuses[]))
			  AND EXISTS (
					SELECT 1
					FROM acls ac
					JOIN roles r ON ac.role_code = r.code
					WHERE ac.user_uuid = $3
					  AND ac.resource_type = 'target'
					  AND ac.resource_uuid = t.uuid
					  AND r.rank <= vc.cutoff
			  )
			UNION ALL
			SELECT 'action'::text AS kind,
					1 as kind_order,
					a.uuid,
					a.title,
					a.description,
					a.status AS status,
					(btrim(COALESCE(a.notes, '')) <> '') AS has_notes,
					a.last_active,
					(COALESCE(ts_rank(af.fts_chinese_tsv, plainto_tsquery('simple', search.chinese)), 0) +
					COALESCE(ts_rank(af.fts_english_tsv, plainto_tsquery('english', search.english)), 0)) AS score
			FROM search
			JOIN actions a ON TRUE
			JOIN actions_fts af ON af.action_uuid = a.uuid
			JOIN viewer_cutoff vc ON TRUE
			WHERE (search.chinese = '' OR af.fts_chinese_tsv @@ plainto_tsquery('simple', search.chinese))
			  AND (search.english = '' OR af.fts_english_tsv @@ plainto_tsquery('english', search.english))
			  AND ($6 = '{}'::statuses[] OR a.status = ANY($6::statuses[]))
			  AND EXISTS (
					SELECT 1
					FROM acls ac
					JOIN roles r ON ac.role_code = r.code
					WHERE ac.user_uuid = $3
					  AND r.rank <= vc.cutoff
					  AND (
							(ac.resource_type = 'action' AND ac.resource_uuid = a.uuid)
						 OR (ac.resource_type = 'target' AND ac.resource_uuid = a.target_uuid)
					  )
			  )
			UNION ALL
				SELECT 'session'::text AS kind,
					2 as kind_order,
					s.uuid,
					''::text AS title,	
					''::text AS description,	
					CASE
						WHEN s.ends_at IS NULL THEN 'in progress'::statuses
						ELSE 'completed'::statuses
					END AS status,
					(btrim(COALESCE(s.notes, '')) <> '') AS has_notes,
					s.updated_at AS last_active,
					(COALESCE(ts_rank(sf.fts_chinese_notes_tsv, plainto_tsquery('simple', search.chinese)), 0) +
					COALESCE(ts_rank(sf.fts_english_notes_tsv, plainto_tsquery('english', search.english)), 0)) AS score
			FROM search
			JOIN sessions s ON TRUE
			JOIN sessions_fts sf ON sf.session_uuid = s.uuid
			JOIN actions a ON s.action_uuid = a.uuid
			JOIN targets t ON a.target_uuid = t.uuid
			JOIN viewer_cutoff vc ON TRUE
			WHERE (search.chinese = '' OR sf.fts_chinese_notes_tsv @@ plainto_tsquery('simple', search.chinese))
			  AND (search.english = '' OR sf.fts_english_notes_tsv @@ plainto_tsquery('english', search.english))
			  AND ($6 = '{}'::statuses[] 
				OR CASE 
					WHEN s.ends_at IS NULL THEN 'in progress'::statuses 
					ELSE 'completed'::statuses 
				END = ANY($6::statuses[])
			  ) AND EXISTS (
					SELECT 1
					FROM acls ac
					JOIN roles r ON ac.role_code = r.code
					WHERE ac.user_uuid = $3
					  AND r.rank <= vc.cutoff
					  AND (ac.resource_type, ac.resource_uuid) IN (
							('session', s.uuid),
							('action', s.action_uuid),
							('target', t.uuid)
					  )
			  )
		)
		SELECT total_count,
			   kind,
			   uuid,
			   COALESCE(title, '') AS title,
			   COALESCE(description, '') AS description,
			   status,
			   has_notes,
			   last_active,
			   score
		FROM (
			SELECT h.*,
				   COUNT(*) OVER() AS total_count
			FROM hits h
		) counted
		ORDER BY kind_order, score DESC, last_active DESC
		LIMIT $4 OFFSET $5;
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{
		token.Chinese,
		token.English,
		userUUID,
		filters.limit(),
		filters.offset(),
		pq.Array(filters.Status),
	}

	rows, err := m.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Metadata{}, err
	}
	defer rows.Close()

	totalRecords := 0
	records := []*Record{}

	for rows.Next() {
		var record Record
		var ignored float64

		err := rows.Scan(
			&totalRecords,
			&record.Kind,
			&record.UUID,
			&record.Title,
			&record.Description,
			&record.Status,
			&record.HasNotes,
			&record.LastActive,
			&ignored,
		)
		if err != nil {
			return nil, Metadata{}, err
		}

		records = append(records, &record)
	}
	if err = rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	metadata := calculateMetadata(totalRecords, filters.Page, filters.PageSize)

	return records, metadata, nil
}
