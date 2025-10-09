package data

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/liuminhaw/sessions-of-life/internal/validator"
)

const (
	ScopeActivation     = "activation"
	ScopeAuthentication = "authentication"
	ScopeRefresh        = "refresh"
	ScopePasswordReset  = "password-reset"
)

// Token struct holds the information for an individual token.
type Token struct {
	Plaintext   string
	Hash        []byte
	UserUUID    uuid.UUID
	SessionUUID uuid.UUID
	Expiry      time.Time
	Scope       string
}

func generateToken(
	userUUID uuid.UUID,
	sessionUUID uuid.UUID,
	ttl time.Duration,
	scope string,
) *Token {
	token := &Token{
		Plaintext:   rand.Text(),
		UserUUID:    userUUID,
		SessionUUID: sessionUUID,
		Expiry:      time.Now().Add(ttl),
		Scope:       scope,
	}

	hash := sha256.Sum256([]byte(token.Plaintext))
	token.Hash = hash[:]

	return token
}

func ValidateTokenPlaintext(v *validator.Validator, tokenPlaintext string) {
	v.Check(tokenPlaintext != "", "token", "must be provided")
	v.Check(len(tokenPlaintext) == 26, "token", "must be 26 bytes long")
}

type TokenModel struct {
	DB DBTX
}

// New() method creates a new token and inserts it into the database tokens table.
func (m TokenModel) New(
	userUUID uuid.UUID,
	sessionUUID uuid.UUID,
	ttl time.Duration,
	scope string,
) (*Token, error) {
	token := generateToken(userUUID, sessionUUID, ttl, scope)

	err := m.Insert(token)
	return token, err
}

func (m TokenModel) Get(tokenPlaintext, scope string) (*Token, error) {
	tokenHash := sha256.Sum256([]byte(tokenPlaintext))

	query := `
		SELECT hash, user_uuid, session_uuid, expiry, scope
		FROM tokens 
		WHERE hash = $1 AND scope = $2 AND expiry > $3`
	args := []any{tokenHash[:], scope, time.Now()}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var token Token
	err := m.DB.QueryRowContext(ctx, query, args...).Scan(
		&token.Hash,
		&token.UserUUID,
		&token.SessionUUID,
		&token.Expiry,
		&token.Scope,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &token, nil
}

func (m TokenModel) Insert(token *Token) error {
	query := `
		INSERT INTO tokens (hash, user_uuid, session_uuid, expiry, scope)
		VALUES ($1, $2, $3, $4, $5)`

	args := []any{token.Hash, token.UserUUID, token.SessionUUID, token.Expiry, token.Scope}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := m.DB.ExecContext(ctx, query, args...)
	return err
}

func (m TokenModel) Delete(tokenPlaintext, scope string) error {
	tokenHash := sha256.Sum256([]byte(tokenPlaintext))

	query := `
		DELETE FROM tokens
		WHERE hash = $1 AND scope = $2`
	args := []any{tokenHash[:], scope}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := m.DB.ExecContext(ctx, query, args...)
	return err
}

// DeleteAllForUser() deletes all tokens for a specific user and scope.
func (m TokenModel) DeleteAllForUser(scope string, userUUID uuid.UUID) error {
	query := `
		DELETE FROM tokens
		WHERE scope = $1 AND user_uuid = $2`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := m.DB.ExecContext(ctx, query, scope, userUUID)
	return err
}

// DeleteAllForUserSession() deletes all tokens for a specific user's session.
func (m TokenModel) DeleteAllForUserSession(userUUID, sessionUUID uuid.UUID) error {
	query := `
		DELETE FROM tokens
		WHERE user_uuid = $1 AND session_uuid = $2`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := m.DB.ExecContext(ctx, query, userUUID, sessionUUID)
	return err
}
