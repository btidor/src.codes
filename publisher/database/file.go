package database

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/btidor/src.codes/publisher/analysis"
)

func (db *Database) DeduplicateFiles(files []analysis.File) []analysis.File {
	var deduped []analysis.File
	for i := 0; i < len(files); i += db.batchSize {
		var values []interface{}
		var query string = "SELECT DISTINCT short_hash FROM files" +
			" WHERE short_hash IN ("
		var n int = 1
		for j := i; j < i+db.batchSize && j < len(files); j++ {
			values = append(values, convertHash(files[j].SHA256))
			query += fmt.Sprintf("$%d, ", n)
			n++
		}

		query = query[:len(query)-2] + ")" // strip trailing comma+space
		rows, err := db.Query(query, values...)
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

func (db *Database) RecordHashes(hashes [][32]byte) {
	// Note: we expect the caller to batch calls to RecordFile appropriately
	if len(hashes) < 1 {
		return
	}

	var values []interface{}
	var query string = "INSERT INTO files (short_hash) VALUES "
	for i, hash := range hashes {
		values = append(values, convertHash(hash))
		query += fmt.Sprintf("($%d), ", i+1)
	}

	query = query[:len(query)-2] +
		" ON CONFLICT (short_hash) DO NOTHING"
	_, err := db.Exec(query, values...)
	if err != nil {
		panic(err)
	}
}

func convertHash(hash [32]byte) int64 {
	v := binary.LittleEndian.Uint64(hash[:8])
	return int64(v)
}
