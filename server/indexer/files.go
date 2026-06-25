package indexer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/rs/zerolog/log"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/files"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/model"
)

var (
	ErrEmptyFile    = errors.New("empty file")
	ErrBinaryFile   = errors.New("binary file")
	ErrFileTooLarge = errors.New("file too large")

	maxFileSize int64 = 1024 * 1024 // 1MB default
)

func IndexAll(dirs []*config.Directory) {
	for _, dir := range dirs {
		expanded := files.ExpandHome(dir.Path)
		if err := indexDirectory(expanded, dir); err != nil {
			log.Error().Err(err).Str("directory", expanded).Msg("Failed to index directory")
		}
	}
}

func indexDirectory(dir string, cfg *config.Directory) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("cannot access directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", dir)
	}

	indexed := 0
	skipped := 0

	var userID uint
	if cfg.User != "" {
		u, err := model.GetUser(cfg.User)
		if err != nil {
			log.Error().Err(err).Str("directory", dir).Msg("Failed to resolve user for directory")
			return fmt.Errorf("user %q not found for directory %s: %w", cfg.User, dir, err)
		}
		userID = u.ID
	}

	log.Debug().Str("directory", dir).Msg("Indexing directory")

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("Error accessing path")
			return nil
		}
		if d.IsDir() {
			if path != dir && files.ShouldSkipDir(d.Name(), cfg.Excludes, cfg.IncludeHidden) {
				return filepath.SkipDir
			}
			return nil
		}
		if !cfg.IsMatching(d.Name()) {
			return nil
		}
		if err := IndexFile(path, userID); err != nil {
			log.Debug().Err(err).Str("path", path).Msg("Skipping file")
			skipped++
		} else {
			indexed++
		}
		return nil
	})

	log.Debug().Str("directory", dir).Int("indexed", indexed).Int("skipped", skipped).Msg("Directory indexing complete")
	return err
}

func IndexFile(path string, userID uint) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return ErrEmptyFile
	}

	if info.Size() > maxFileSize {
		return fmt.Errorf("%w: %d bytes", ErrFileTooLarge, info.Size())
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	fileURL := files.PathToFileURL(absPath)

	// Skip if already indexed with the same modification time
	existing := GetByURLAndUser(fileURL, userID)
	if existing != nil && existing.Added == info.ModTime().Unix() {
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return &document.ReadFileError{
			Msg: err.Error(),
		}
	}

	doc := &document.Document{
		URL:    fileURL,
		Added:  info.ModTime().Unix(),
		UserID: userID,
	}

	if strings.EqualFold(filepath.Ext(path), ".pdf") {
		return AddPDF(doc, content)
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".md" || ext == ".markdown" {
		return AddMarkdown(doc, content)
	}

	if strings.EqualFold(filepath.Ext(path), ".org") {
		return AddOrg(doc, content)
	}

	if !utf8.Valid(content) {
		return ErrBinaryFile
	}
	if int64(len(content)) > maxFileSize {
		return fmt.Errorf("%w: %d bytes", ErrFileTooLarge, int64(len(content)))
	}

	doc.Text = string(content)
	return i.AddDocument(doc)
}

// DeleteFile removes the document for the given filesystem path from the index.
// It uses a url: field query so it removes the file across all users and
// language-specific sub-indexes. Returns nil if the document is not found.
func DeleteFile(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}
	fileURL := files.PathToFileURL(absPath)
	_, err = DeleteByQuery("url:"+fileURL, nil, nil)
	return err
}
