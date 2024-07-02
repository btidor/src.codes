package database

import (
	"database/sql"

	_ "github.com/jackc/pgx/v4/stdlib"
)

type Database struct {
	*sql.DB
	batchSize int
}

func Connect(conn string, batchSize int) (*Database, error) {
	db, err := sql.Open("pgx", conn)
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Database{db, batchSize}, nil
}
