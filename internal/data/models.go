package data

import (
	"database/sql"
	"errors"

	"github.com/yanyiwu/gojieba"
)

// ErrRecordNotFound will be returned when a record is not found in the database.
var (
	ErrRecordNotFound = errors.New("record not found")
	ErrEditConflict   = errors.New("edit conflict")
)

type Models struct {
	Targets    TargetModel
	Activities ActivityModel
	Tokens     TokenModel
	Users      UserModel
}

// NewModels returns a Models struct containing the initialized TargetModel.
func NewModels(db *sql.DB, jieba *gojieba.Jieba) Models {
	return Models{
		Targets:    TargetModel{DB: db, Jieba: jieba},
		Activities: ActivityModel{DB: db, Jieba: jieba},
		Tokens:     TokenModel{DB: db},
		Users:      UserModel{DB: db},
	}
}
