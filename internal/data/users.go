package data

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"database/sql"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/gofrs/uuid/v5"
	"github.com/liuminhaw/sessions-of-life/internal/validator"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/unicode/norm"
)

const (
	bcryptCost                          = 12
	users_email_key_duplicate_violation = `pq: duplicate key value violates unique constraint "users_email_key"`
)

var ErrDuplicateEmail = errors.New("duplicate email")

var AnonymousUser = &User{}

type User struct {
	UUID      uuid.UUID `json:"uuid"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  password  `json:"-"`
	Activated bool      `json:"activated"`
	Version   int       `json:"-"`
}

func (u *User) IsAnonymous() bool {
	return u == AnonymousUser
}

type password struct {
	// Using a pointer to string to distinguish between a plaintext password not
	// presented (nil) and an empty string.
	plaintext *string
	hash      []byte
}

// Set() method calculate the bcrypt hash of the plaintext password and stores
// both the plaintext and the hash in the password struct.
func (p *password) Set(plaintextPassword, pepper string) error {
	pw := norm.NFKC.String(plaintextPassword)

	// Hash password to fixed length with pepper before actual bcrypt hashing.
	mac := hmac.New(sha512.New, []byte(pepper))
	mac.Write([]byte(pw))
	preHash := mac.Sum(nil)

	hash, err := bcrypt.GenerateFromPassword([]byte(preHash), bcryptCost)
	if err != nil {
		return err
	}

	p.plaintext = &plaintextPassword
	p.hash = hash

	return nil
}

// Matches() method checks if the provided plaintext password matches the stored hash.
func (p *password) Matches(plaintextPassword, pepper string) (bool, error) {
	pw := norm.NFKC.String(plaintextPassword)

	// Hash password to fixed length with pepper before actual bcrypt comparison.
	mac := hmac.New(sha512.New, []byte(pepper))
	mac.Write([]byte(pw))
	preHash := mac.Sum(nil)

	err := bcrypt.CompareHashAndPassword(p.hash, []byte(preHash))
	if err != nil {
		switch {
		case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
			return false, nil
		default:
			return false, err
		}
	}

	return true, nil
}

func ValidateEmail(v *validator.Validator, email string) {
	v.Check(email != "", "email", "must be provided")
	v.Check(validator.Matches(email, validator.EmailRX), "email", "must be a valid email address")
}

func ValidatePasswordPlaintext(v *validator.Validator, password string) {
	pw := norm.NFKC.String(password)

	v.Check(pw != "", "password", "must be provided")
	v.Check(
		validator.ValidUnicodeChars(pw),
		"password",
		"must no contain unicode Control or Format characters",
	)
	v.Check(utf8.RuneCountInString(password) >= 8, "password", "must be at least 8 bytes long")
	// Consider: pre-hash the password to not enforce the length limit
	v.Check(
		utf8.RuneCountInString(password) <= 72,
		"password",
		"must not be more than 72 bytes long",
	)
}

func ValidateUser(v *validator.Validator, user *User) {
	v.Check(user.Name != "", "name", "must be provided")
	v.Check(utf8.RuneCountInString(user.Name) <= 30, "name", "must not be more than 40 bytes long")

	ValidateEmail(v, user.Email)
	if user.Password.plaintext != nil {
		ValidatePasswordPlaintext(v, *user.Password.plaintext)
	}

	if user.Password.hash == nil {
		panic("missing password hash for user")
	}
}

type UserModel struct {
	DB *sql.DB
}

func (m UserModel) Insert(user *User) error {
	query := `
		INSERT INTO users (name, email, password_hash, activated)
		VALUES ($1, $2, $3, $4)
		RETURNING uuid, created_at, updated_at, version`

	args := []any{user.Name, user.Email, user.Password.hash, user.Activated}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).
		Scan(&user.UUID, &user.CreatedAt, &user.UpdatedAt, &user.Version)
	if err != nil {
		switch {
		case err.Error() == users_email_key_duplicate_violation:
			return ErrDuplicateEmail
		default:
			return err
		}
	}

	return nil
}

func (m UserModel) GetByEmail(email string) (*User, error) {
	query := `
		SELECT uuid, created_at, updated_at, name, email, password_hash, activated, version
		FROM users
		WHERE email = $1`

	var user User

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, email).Scan(
		&user.UUID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.Name,
		&user.Email,
		&user.Password.hash,
		&user.Activated,
		&user.Version,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &user, nil
}

func (m UserModel) Update(user *User) error {
	query := `
		UPDATE users
		SET name = $1, email = $2, password_hash = $3, activated = $4, updated_at = now(), version = version + 1
		WHERE uuid = $5 AND version = $6
		RETURNING version`

	args := []any{
		user.Name,
		user.Email,
		user.Password.hash,
		user.Activated,
		user.UUID,
		user.Version,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&user.Version)
	if err != nil {
		switch {
		case err.Error() == users_email_key_duplicate_violation:
			return ErrDuplicateEmail
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}

	return nil
}

func (m UserModel) GetForToken(tokenScope, tokenPlaintext string) (*User, error) {
	tokenHash := sha256.Sum256([]byte(tokenPlaintext))

	query := `
		SELECT
			users.uuid, 
			users.created_at, 
			users.updated_at, 
			users.name, 
			users.email, 
			users.password_hash, 
			users.activated, 
			users.version
		FROM users
		INNER JOIN tokens ON users.uuid = tokens.user_uuid
		WHERE tokens.hash = $1 AND tokens.scope = $2 AND tokens.expiry > $3`
	args := []any{tokenHash[:], tokenScope, time.Now()}

	var user User
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(
		&user.UUID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.Name,
		&user.Email,
		&user.Password.hash,
		&user.Activated,
		&user.Version,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &user, nil
}
