package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/liuminhaw/yatijapp/internal/validator"
)

type filter struct {
	SortBy    string   `json:"sortBy"`
	SortOrder string   `json:"sortOrder"`
	Status    []Status `json:"status"`
}

type filters struct {
	Target  filter `json:"target"`
	Action  filter `json:"action"`
	Session filter `json:"session"`
}

type Preferences struct {
	Filters filters `json:"filters"`
	Version string  `json:"version"`
}

func ValidatePreferences(v *validator.Validator, p *Preferences) {
	// Preferences version
	v.Check(p.Version == "2025-12-09", "version", "must be '2025-12-09'")
	// SortBy
	v.Check(
		validator.PermittedValue(p.Filters.Target.SortBy, SortSafelist...),
		"filters.target.sortBy",
		"not a permitted value",
	)
	v.Check(
		validator.PermittedValue(p.Filters.Action.SortBy, SortSafelist...),
		"filters.action.sortBy",
		"not a permitted value",
	)
	v.Check(
		validator.PermittedValue(p.Filters.Session.SortBy, SessionSortSafelist...),
		"filters.session.sortBy",
		"not a permitted value",
	)
	// SortOrder
	v.Check(
		validator.PermittedValue(p.Filters.Target.SortOrder, SortOrderSafelist...),
		"filters.target.sortOrder",
		"not a permitted value",
	)
	v.Check(
		validator.PermittedValue(p.Filters.Action.SortOrder, SortOrderSafelist...),
		"filters.action.sortOrder",
		"not a permitted value",
	)
	v.Check(
		validator.PermittedValue(p.Filters.Session.SortOrder, SortOrderSafelist...),
		"filters.session.sortOrder",
		"not a permitted value",
	)
	// Status
	v.Check(p.Filters.Target.Status != nil, "filters.target.status", "must be provided")
	v.Check(p.Filters.Action.Status != nil, "filters.action.status", "must be provided")
	v.Check(p.Filters.Session.Status != nil, "filters.session.status", "must be provided")
	v.Check(
		validator.PermittedValues(p.Filters.Target.Status, StatusSafelist...),
		"filters.target.status",
		"contains invalid status value",
	)
	v.Check(
		validator.PermittedValues(p.Filters.Action.Status, StatusSafelist...),
		"filters.action.status",
		"contains invalid status value",
	)
	v.Check(
		validator.PermittedValues(p.Filters.Session.Status, SessionStatusSafelist...),
		"filters.action.status",
		"contains invalid status value",
	)
}

type UserPreferences struct {
	Filters  []byte
	UserUUID uuid.UUID
}

type UserPreferencesModel struct {
	DB DBTX
}

// func (upm UserPreferencesModel) Get(userUUID uuid.UUID) (*UserPreferences, error) {
func (upm UserPreferencesModel) Get(userUUID uuid.UUID) (*Preferences, error) {
	query := `
		SELECT preference from preferences
		WHERE user_uuid = $1
	`

	var prefb UserPreferences

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := upm.DB.QueryRowContext(ctx, query, userUUID).Scan(&prefb.Filters)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	var pref Preferences
	err = json.Unmarshal(prefb.Filters, &pref)
	if err != nil {
		return nil, err
	}

	return &pref, nil
}

func (upm UserPreferencesModel) Put(userUUID uuid.UUID, data []byte) error {
	query := `
		INSERT INTO preferences (user_uuid, preference)
		VALUES ($1, $2::jsonb)
		ON CONFLICT (user_uuid) DO UPDATE
		SET preference = $2
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// _, err := upm.DB.ExecContext(ctx, query, up.UserUUID, up.Filters)
	_, err := upm.DB.ExecContext(ctx, query, userUUID, data)
	if err != nil {
		return err
	}

	return nil
}
