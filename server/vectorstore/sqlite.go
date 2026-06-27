// SPDX-License-Identifier: AGPL-3.0-or-later

package vectorstore

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/server/vectorstore/sqlitevec"

	"github.com/rs/zerolog/log"
)

type sqliteVectorStore struct {
	db         *sql.DB
	dimensions int
}

func newSQLite(cfg *config.Config) (VectorStore, error) {
	sqlitevec.Auto()
	dbPath := cfg.FullPath(cfg.Server.Database)
	dir := filepath.Dir(dbPath)
	vecDBPath := filepath.Join(dir, "vectors.sqlite3")

	db, err := sql.Open("sqlite3", vecDBPath)
	if err != nil {
		return nil, fmt.Errorf("open vector database: %w", err)
	}
	// Single connection to avoid locking issues with SQLite.
	db.SetMaxOpenConns(1)

	return &sqliteVectorStore{
		db:         db,
		dimensions: cfg.SemanticSearch.Dimensions,
	}, nil
}

func (s *sqliteVectorStore) Init() error {
	// Verify sqlite-vec is available by querying its version.
	var version string
	if err := s.db.QueryRow("SELECT vec_version()").Scan(&version); err != nil {
		return fmt.Errorf("sqlite-vec extension not available (is the vec0 shared library installed?): %w", err)
	}
	log.Info().Str("version", version).Msg("sqlite-vec loaded")

	// Regular table for chunk metadata (text content, doc association).
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS chunk_meta (
		chunk_key TEXT PRIMARY KEY,
		doc_id TEXT NOT NULL,
		chunk_idx INTEGER NOT NULL,
		user_id INTEGER NOT NULL DEFAULT 0,
		chunk_text TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create chunk_meta table: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_chunk_meta_doc ON chunk_meta(doc_id)`); err != nil {
		return fmt.Errorf("create chunk_meta doc_id index: %w", err)
	}

	// Vec0 virtual table for vector similarity search.
	stmt := fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS embeddings USING vec0(
		user_id INTEGER PARTITION KEY,
		chunk_key TEXT PRIMARY KEY,
		embedding FLOAT[%d]
	)`, s.dimensions)
	if _, err := s.db.Exec(stmt); err != nil {
		return fmt.Errorf("create embeddings table: %w", err)
	}
	return nil
}

func chunkKey(docID string, chunkIdx int) string {
	return fmt.Sprintf("%s#%d", docID, chunkIdx)
}

func (s *sqliteVectorStore) PutChunks(docID string, userID uint, chunks []Chunk) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Delete all existing chunks for this document.
	if _, err = tx.Exec(`DELETE FROM embeddings WHERE chunk_key IN (SELECT chunk_key FROM chunk_meta WHERE doc_id = ?)`, docID); err != nil {
		return fmt.Errorf("delete old embeddings: %w", err)
	}
	if _, err = tx.Exec(`DELETE FROM chunk_meta WHERE doc_id = ?`, docID); err != nil {
		return fmt.Errorf("delete old chunk_meta: %w", err)
	}

	metaStmt, err := tx.Prepare(`INSERT INTO chunk_meta(chunk_key, doc_id, chunk_idx, user_id, chunk_text) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare chunk_meta insert: %w", err)
	}
	defer metaStmt.Close() //nolint:errcheck

	embStmt, err := tx.Prepare(`INSERT INTO embeddings(user_id, chunk_key, embedding) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare embeddings insert: %w", err)
	}
	defer embStmt.Close() //nolint:errcheck

	for _, c := range chunks {
		key := chunkKey(docID, c.Index)
		if _, err = metaStmt.Exec(key, docID, c.Index, userID, c.Text); err != nil {
			return fmt.Errorf("insert chunk_meta: %w", err)
		}
		blob := float32ToBlob(c.Embedding)
		if _, err = embStmt.Exec(userID, key, blob); err != nil {
			return fmt.Errorf("insert embedding: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit chunks: %w", err)
	}
	return nil
}

func (s *sqliteVectorStore) Delete(docID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.Exec(`DELETE FROM embeddings WHERE chunk_key IN (SELECT chunk_key FROM chunk_meta WHERE doc_id = ?)`, docID); err != nil {
		return fmt.Errorf("delete embeddings: %w", err)
	}
	if _, err = tx.Exec(`DELETE FROM chunk_meta WHERE doc_id = ?`, docID); err != nil {
		return fmt.Errorf("delete chunk_meta: %w", err)
	}
	return tx.Commit()
}

func (s *sqliteVectorStore) Search(vector []float32, topK int, threshold float64, userID uint) (_ []Result, err error) {
	userResults, err := s.searchUser(vector, topK, threshold, userID)
	if err != nil || userID == 0 {
		return userResults, err
	}
	globalResults, err := s.searchUser(vector, topK, threshold, 0)
	if err != nil {
		return nil, err
	}
	return mergeSearchResults(topK, userResults, globalResults), nil
}

func (s *sqliteVectorStore) searchUser(vector []float32, topK int, threshold float64, userID uint) (_ []Result, err error) {
	blob := float32ToBlob(vector)
	rows, err := s.db.Query(
		`SELECT e.chunk_key, e.distance, COALESCE(m.doc_id, ''), COALESCE(m.chunk_idx, 0), COALESCE(m.chunk_text, '')
		 FROM embeddings e
		 LEFT JOIN chunk_meta m ON e.chunk_key = m.chunk_key
		 WHERE e.embedding MATCH ?
		   AND e.k = ?
		   AND e.user_id = ?
		 ORDER BY e.distance`,
		blob, topK, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("vector search for user %d: %w", userID, err)
	}
	defer func() {
		if cerr := rows.Close(); err == nil {
			err = cerr
		}
	}()

	var results []Result
	for rows.Next() {
		var chunkKey, docID, chunkText string
		var chunkIdx int
		var distance float64
		if err := rows.Scan(&chunkKey, &distance, &docID, &chunkIdx, &chunkText); err != nil {
			return nil, fmt.Errorf("scan vector result: %w", err)
		}
		similarity := 1.0 - distance
		if similarity >= threshold {
			results = append(results, Result{
				DocID:      docID,
				ChunkIdx:   chunkIdx,
				ChunkText:  chunkText,
				Similarity: similarity,
			})
		}
	}
	return results, rows.Err()
}

func (s *sqliteVectorStore) Clear() error {
	if _, err := s.db.Exec(`DELETE FROM embeddings`); err != nil {
		return fmt.Errorf("clear embeddings: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM chunk_meta`); err != nil {
		return fmt.Errorf("clear chunk_meta: %w", err)
	}
	return nil
}

func (s *sqliteVectorStore) Close() error {
	return s.db.Close()
}

// float32ToBlob converts a []float32 to a little-endian byte slice suitable
// for sqlite-vec's vec_f32 format.
func float32ToBlob(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}
