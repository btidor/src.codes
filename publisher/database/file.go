package database

import (
	"encoding/hex"
	"strings"

	"github.com/btidor/src.codes/publisher/analysis"
)

func (db *Database) DeduplicateFiles(files []analysis.File) []analysis.File {
	var deduped []analysis.File
	for i := 0; i < len(files); i += db.batchSize {
		var values []interface{}
		for j := i; j < i+db.batchSize && j < len(files); j++ {
			values = append(values, files[j].SHA256[:8])
		}

		rows, err := db.Query(
			"SELECT DISTINCT short_hash FROM files WHERE short_hash IN (?"+
				strings.Repeat(", ?", len(values)-1)+")",
			values...,
		)
		if err != nil {
			panic(err)
		}

		existing := make(map[string]bool, db.batchSize)
		for rows.Next() {
			var hash []byte
			if err := rows.Scan(&hash); err != nil {
				panic(err)
			}
			existing[hex.EncodeToString(hash)] = true
		}

		for j := i; j < i+db.batchSize && j < len(files); j++ {
			h := hex.EncodeToString(files[j].SHA256[:8])
			if _, found := existing[h]; !found {
				deduped = append(deduped, files[j])
			}
		}
	}
	return deduped
}

func (db *Database) RecordFiles(files []analysis.File) {
	// Note: we expect the caller to batch calls to RecordFile appropriately
	if len(files) < 1 {
		return
	}

	var values []interface{}
	for i := 0; i < len(files); i++ {
		values = append(values, files[i].SHA256[:8])
	}

	_, err := db.Exec(
		"INSERT INTO files (short_hash) VALUES (?)"+
			strings.Repeat(", (?)", len(values)-1)+
			" ON DUPLICATE KEY UPDATE short_hash = VALUES(short_hash)",
		values...,
	)
	if err != nil {
		panic(err)
	}
}
