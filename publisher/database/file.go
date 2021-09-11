package database

import (
	"strings"

	"github.com/btidor/src.codes/publisher/analysis"
)

func (db *Database) DeduplicateFiles(files []analysis.File) []analysis.File {
	var deduped []analysis.File
	for i := 0; i < len(files); i += db.batchSize {
		var hashes []interface{}
		for j := i; j < i+db.batchSize && j < len(files); j++ {
			hashes = append(hashes, files[j].ShortHash())
		}

		rows, err := db.Query(
			"SELECT DISTINCT short_hash FROM files WHERE file_hash IN (?"+
				strings.Repeat(", ?", len(hashes)-1)+")",
			hashes...,
		)
		if err != nil {
			panic(err)
		}

		var hash [8]byte
		existing := make(map[[8]byte]bool, db.batchSize)
		for rows.Next() {
			if err := rows.Scan(&hash); err != nil {
				panic(err)
			}
			existing[hash] = true
		}

		for j := i; j < i+db.batchSize && j < len(files); j++ {
			h := files[j].ShortHash()
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
		values = append(values, files[i].ShortHash())
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
