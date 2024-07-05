package database

import (
	"database/sql"
	"errors"
	"os"

	_ "embed"

	_ "github.com/mattn/go-sqlite3"
)

type Database struct {
	*sql.DB
	batchSize int
}

//go:embed schema.sql
var create string

func Connect(filename string, batchSize int) (*Database, error) {
	_, err := os.Stat(filename)
	first := errors.Is(err, os.ErrNotExist)

	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}

	if first {
		_, err = db.Exec(create)
		if err != nil {
			return nil, err
		}
	}

	err = db.Ping()
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Database{db, batchSize}, nil
}
