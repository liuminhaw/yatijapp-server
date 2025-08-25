package data

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/liuminhaw/sessions-of-life/internal/validator"
)

const (
	ScopeActivation     = "activation"
	ScopeAuthentication = "authentication"
	ScopeRefresh        = "refresh"
)

// Token struct holds the information for an individual token.
type Token struct {
	Plaintext string
	Hash      []byte
	UserUUID  uuid.UUID
	Expiry    time.Time
	Scope     string
}

func generateToken(userUUID uuid.UUID, ttl time.Duration, scope string) *Token {
	token := &Token{
		Plaintext: rand.Text(),
		UserUUID:  userUUID,
		Expiry:    time.Now().Add(ttl),
		Scope:     scope,
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
	DB *sql.DB
}

// New() method creates a new token and inserts it into the database tokens table.
func (m TokenModel) New(userUUID uuid.UUID, ttl time.Duration, scope string) (*Token, error) {
	token := generateToken(userUUID, ttl, scope)

	err := m.Insert(token)
	return token, err
}

func (m TokenModel) Insert(token *Token) error {
	query := `
		INSERT INTO tokens (hash, user_uuid, expiry, scope)
		VALUES ($1, $2, $3, $4)`

	args := []any{token.Hash, token.UserUUID, token.Expiry, token.Scope}
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

