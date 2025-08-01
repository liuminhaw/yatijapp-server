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
	UUID      uuid.UUID `json:"uuid"`
	CreatedAt time.Time `json:"created_at"`
	DueAt     time.Time `json:"due_at,omitzero"`
	UpdatedAt time.Time `json:"updated_at"`
	// CompletedAt time.Time `json:"completed_at,omitzero"`
	Title       string `json:"title"`
	Description string `json:"description,omitzero"`
	Notes       string `json:"notes,omitzero"`
	Version     int32  `json:"version"`
	Status      Status `json:"status,omitzero"` // e.g., "queued", "in progress", "complete", "canceled"
	SerialID    int64  `json:"-"`               // Optional field for serial ID, not used in all contexts
}

func ValidateTarget(v *validator.Validator, target *Target) {
	v.Check(target.Title != "", "title", "must be provided")
	v.Check(len(target.Title) <= 200, "title", "must not be more than 200 characters long")
	v.Check(target.Status != "", "status", "must be provided")
	v.Check(
		validator.PermittedValue(target.Status, StatusSafelist...),
		"status",
		"must be one of 'queued', 'in progress', 'complete', or 'canceled'",
	)
	v.Check(target.DueAt.After(time.Now()), "due_at", "must be in the future")
}

type TargetFTS struct {
	// UUID             uuid.UUID
	TitleToken       *tokenizer.Tokenizer
	DescriptionToken *tokenizer.Tokenizer
	NotesToken       *tokenizer.Tokenizer
	// TitleChineseTSVector       string `json:"title_chinese_tsv"`
	// TitleEnglishTSVector       string `json:"title_english_tsv"`
	// DescriptionChineseTSVector string `json:"description_chinese_tsv"`
	// DescriptionEnglishTSVector string `json:"description_english_tsv"`
}

func GenTargetFTS(title, description, notes string, jieba *gojieba.Jieba) TargetFTS {
	titleTokenizer := tokenizer.New(title, jieba)
	descriptionTokenizer := tokenizer.New(description, jieba)
	notesTokenizer := tokenizer.New(notes, jieba)

	return TargetFTS{
		TitleToken:       titleTokenizer,
		DescriptionToken: descriptionTokenizer,
		NotesToken:       notesTokenizer,
	}
}

// TargetModel struct type wraps a sql.DB connection pool.
type TargetModel struct {
	DB    *sql.DB
	Jieba *gojieba.Jieba
}

func (t TargetModel) Insert(target *Target, fts TargetFTS) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	tx, err := t.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	args := []any{target.Title, target.Description, target.Notes, target.DueAt, target.Status}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO targets (title, description, notes, due_at, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING uuid, created_at, updated_at, version`, args...,
	).Scan(&target.UUID, &target.CreatedAt, &target.UpdatedAt, &target.Version)
	if err != nil {
		return err
	}

	args = []any{
		target.UUID,
		fts.TitleToken.Chinese,
		fts.DescriptionToken.Chinese,
		fts.NotesToken.Chinese,
		fts.TitleToken.English,
		fts.DescriptionToken.English,
		fts.NotesToken.English,
	}
	// _, err = tx.ExecContext(ctx, `
	// 	INSERT INTO targets_fts (
	// 			target_uuid,
	// 			title_chinese_tsv,
	// 			title_english_tsv,
	// 			description_chinese_tsv,
	// 			description_english_tsv
	// 	) VALUES (
	// 		$1, to_tsvector('simple', $2), to_tsvector('english', $3),
	// 		to_tsvector('simple', $4), to_tsvector('english', $5)
	// )`, args...)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO targets_fts (target_uuid, fts_chinese_tsv, fts_english_tsv)
		VALUES (
			$1,
			setweight(to_tsvector('simple', $2), 'A') ||
			setweight(to_tsvector('simple', $3), 'B') ||
			setweight(to_tsvector('simple', $4), 'C'),
			setweight(to_tsvector('english', $5), 'A') ||
			setweight(to_tsvector('english', $6), 'B') ||
			setweight(to_tsvector('english', $7), 'C')
		)`, args...)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
	// query := `
	//        INSERT INTO targets (title, description, notes, due_at, status)
	// 	VALUES ($1, $2, $3, $4, $5)
	// 	RETURNING uuid, created_at, updated_at, version`
	//
	// args := []any{target.Title, target.Description, target.Notes, target.DueAt, target.Status}
	//
	// ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	// defer cancel()
	//
	// return t.DB.QueryRowContext(ctx, query, args...).
	// 	Scan(&target.UUID, &target.CreatedAt, &target.UpdatedAt, &target.Version)
}

func (t TargetModel) Get(uuid uuid.UUID) (*Target, error) {
	query := `
		SELECT 
			uuid, 
			created_at, 
			due_at, 
			updated_at, 
			title, 
			description, 
			notes, 
			status, 
			version
		FROM targets
		WHERE uuid = $1`

	var target Target

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := t.DB.QueryRowContext(ctx, query, uuid).Scan(
		&target.UUID,
		&target.CreatedAt,
		&target.DueAt,
		&target.UpdatedAt,
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

func (t TargetModel) Update(target *Target) error {
	query := `
		UPDATE targets
		SET title = $1, description = $2, notes = $3, 
			due_at = $4, status = $5, version = version + 1, updated_at = NOW()
		WHERE uuid = $6 AND version = $7
		RETURNING created_at, updated_at, version`

	args := []any{
		target.Title,
		target.Description,
		target.Notes,
		target.DueAt,
		target.Status,
		target.UUID,
		target.Version,
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

func (t TargetModel) Delete(uuid uuid.UUID) error {
	query := `
		DELETE FROM targets
		WHERE uuid = $1`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := t.DB.ExecContext(ctx, query, uuid)
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

func (t TargetModel) GetAll(
	token tokenizer.Tokenizer,
	filters Filters,
) ([]*Target, Metadata, error) {
	// query := `
	// 	SELECT
	// 		uuid,
	// 		created_at,
	// 		due_at,
	// 		updated_at,
	// 		title,
	// 		description,
	// 		notes,
	// 		status,
	// 		version,
	// 		serial_id
	// 	FROM targets
	// 	WHERE (to_tsvector('simple',title) @@ plainto_tsquery('simple',$1) OR $1 = '')
	// 		AND (
	// 			CASE
	// 				WHEN $2 = '' THEN TRUE
	// 				ELSE status = $2::statuses
	// 			END
	// 		)
	// 	ORDER BY serial_id`
	query := fmt.Sprintf(`
		SELECT 
			count(*) OVER(),
			t.uuid, 
			t.created_at, 
			t.due_at, 
			t.updated_at, 
			t.title, 
			t.description, 
			t.notes, 
			t.status, 
			t.version, 
			t.serial_id,
			ts_rank(fts.fts_chinese_tsv, plainto_tsquery('simple', $1))
				+ ts_rank(fts.fts_english_tsv, plainto_tsquery('english', $2)) AS rank
		FROM targets_fts fts
		JOIN targets t ON fts.target_uuid = t.uuid
		WHERE (fts.fts_chinese_tsv @@ plainto_tsquery('simple', $1) OR $1 = '')
			AND (fts.fts_english_tsv @@ plainto_tsquery('english', $2) OR $2 = '')
			AND (
				CASE
					WHEN $3 = '' THEN TRUE
					ELSE status = $3::statuses
				END
			)
		ORDER BY %s %s, rank DESC, serial_id DESC
		LIMIT $4 OFFSET $5`, filters.sortColumn(), filters.sortDirection())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{token.Chinese, token.English, filters.Status, filters.limit(), filters.offset()}

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
			&target.DueAt,
			&target.UpdatedAt,
			&target.Title,
			&target.Description,
			&target.Notes,
			&target.Status,
			&target.Version,
			&target.SerialID,
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
