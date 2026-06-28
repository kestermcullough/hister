package indexer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/model"

	"github.com/blevesearch/bleve/v2"
)

func setupTestDB(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{
		Server: config.Server{
			Database: "file::memory:",
		},
	}
	err := model.Init(cfg)
	if err != nil {
		t.Fatalf("failed to init test DB: %v", err)
	}
	return cfg
}

func TestDirectoryUserResolution(t *testing.T) {
	setupTestDB(t)

	// Create test users
	u1, err := model.CreateUser("alice", "password123", false)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	u2, err := model.CreateUser("bob", "password123", false)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	tests := []struct {
		name     string
		username string
		wantID   uint
		wantErr  bool
	}{
		{
			name:     "empty username is global",
			username: "",
			wantID:   0,
			wantErr:  false,
		},
		{
			name:     "existing user alice",
			username: "alice",
			wantID:   u1.ID,
			wantErr:  false,
		},
		{
			name:     "existing user bob",
			username: "bob",
			wantID:   u2.ID,
			wantErr:  false,
		},
		{
			name:     "non-existent user",
			username: "charlie",
			wantID:   0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotID uint
			var err error
			if tt.username != "" {
				u, e := model.GetUser(tt.username)
				if e != nil {
					err = e
				} else {
					gotID = u.ID
				}
			}
			if tt.wantErr {
				if err == nil {
					t.Errorf("user resolution(%q) expected error, got nil", tt.username)
				}
				return
			}
			if err != nil {
				t.Errorf("user resolution(%q) unexpected error: %v", tt.username, err)
				return
			}
			if gotID != tt.wantID {
				t.Errorf("user resolution(%q) = %d, want %d", tt.username, gotID, tt.wantID)
			}
		})
	}
}

func TestIndexFileWithUserID(t *testing.T) {
	setupTestDB(t)

	// Create a test user
	u, err := model.CreateUser("testuser", "password123", false)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Create temp dirs for data and test files
	dataDir := t.TempDir()
	testDir := t.TempDir()

	// Create test files (avoid patterns that match sensitive content regex)
	testFile := filepath.Join(testDir, "test.txt")
	err = os.WriteFile(testFile, []byte("sample document content about indexing files for testing purposes"), 0o644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	testFile2 := filepath.Join(testDir, "test2.txt")
	err = os.WriteFile(testFile2, []byte("sample global document content for indexing test purposes"), 0o644)
	if err != nil {
		t.Fatalf("failed to create test file 2: %v", err)
	}

	// Initialize the indexer with proper data and index dirs
	idxCfg := config.CreateDefaultConfig()
	idxCfg.App.Directory = dataDir
	// Override the index dir
	err = Init(idxCfg)
	if err != nil {
		t.Fatalf("failed to init indexer: %v", err)
	}
	defer i.Close()

	// Index the file with the test user's ID
	err = IndexFile(testFile, u.ID)
	if err != nil {
		t.Fatalf("IndexFile with user ID failed: %v", err)
	}

	// Index the file without user ID (global)
	err = IndexFile(testFile2, 0)
	if err != nil {
		t.Fatalf("IndexFile without user ID failed: %v", err)
	}
}

func TestAddDocumentIncrementsAddCount(t *testing.T) {
	dataDir := t.TempDir()
	idxCfg := config.CreateDefaultConfig()
	idxCfg.App.Directory = dataDir
	err := Init(idxCfg)
	if err != nil {
		t.Fatalf("failed to init indexer: %v", err)
	}
	defer i.Close()

	url := "https://example.com/count"
	for range 2 {
		err = Add(&document.Document{
			URL:   url,
			Title: "Counted",
			Text:  "Counted document text",
		})
		if err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}

	got := GetByURLAndUser(url, 0)
	if got == nil {
		t.Fatal("document not found")
	}
	if got.AddCount != 2 {
		t.Fatalf("AddCount = %d, want 2", got.AddCount)
	}

	latest := GetLatestDocuments(10, "", 0)
	if latest == nil {
		t.Fatal("latest documents not found")
	}
	if len(latest.Documents) != 1 {
		t.Fatalf("latest documents count = %d, want 1", len(latest.Documents))
	}
	if latest.Documents[0].AddCount != 2 {
		t.Fatalf("latest AddCount = %d, want 2", latest.Documents[0].AddCount)
	}
}

func TestAddDocumentTreatsMissingAddCountAsOne(t *testing.T) {
	dataDir := t.TempDir()
	idxCfg := config.CreateDefaultConfig()
	idxCfg.App.Directory = dataDir
	err := Init(idxCfg)
	if err != nil {
		t.Fatalf("failed to init indexer: %v", err)
	}
	defer i.Close()

	url := "https://example.com/legacy-count"
	err = i.save(&document.Document{
		URL:   url,
		Title: "Legacy counted",
		Text:  "Legacy counted document text",
	})
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	got := GetByURLAndUser(url, 0)
	if got == nil {
		t.Fatal("document not found")
	}
	if got.AddCount != 1 {
		t.Fatalf("legacy AddCount = %d, want 1", got.AddCount)
	}

	err = Add(&document.Document{
		URL:   url,
		Title: "Legacy counted",
		Text:  "Legacy counted document text",
	})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	got = GetByURLAndUser(url, 0)
	if got == nil {
		t.Fatal("document not found after add")
	}
	if got.AddCount != 2 {
		t.Fatalf("AddCount after add = %d, want 2", got.AddCount)
	}
}

func TestSaveRemovesStaleLanguageCopy(t *testing.T) {
	dataDir := t.TempDir()
	idxCfg := config.CreateDefaultConfig()
	idxCfg.App.Directory = dataDir
	err := Init(idxCfg)
	if err != nil {
		t.Fatalf("failed to init indexer: %v", err)
	}
	defer i.Close()

	url := "https://example.com/language-copy"
	err = i.save(&document.Document{
		URL:      url,
		Title:    "Language copy",
		Text:     "Language copy text",
		Language: "en",
		AddCount: 4,
	})
	if err != nil {
		t.Fatalf("first save failed: %v", err)
	}
	if copies := countDocIDCopies(t, document.GetDocID(0, url)); copies != 1 {
		t.Fatalf("copies after first save = %d, want 1", copies)
	}

	err = i.save(&document.Document{
		URL:      url,
		Title:    "Language copy",
		Text:     "Language copy text",
		Language: "",
		AddCount: 5,
	})
	if err != nil {
		t.Fatalf("second save failed: %v", err)
	}
	if copies := countDocIDCopies(t, document.GetDocID(0, url)); copies != 1 {
		t.Fatalf("copies after language change = %d, want 1", copies)
	}

	got := GetByURLAndUser(url, 0)
	if got == nil {
		t.Fatal("document not found")
	}
	if got.AddCount != 5 {
		t.Fatalf("AddCount = %d, want 5", got.AddCount)
	}
}

func countDocIDCopies(t *testing.T, id string) uint64 {
	t.Helper()
	q := bleve.NewDocIDQuery([]string{id})
	req := bleve.NewSearchRequest(q)
	req.Size = 10
	res, err := i.idx.Search(req)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	return res.Total
}

func TestDirectoryUserField(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		expected string
	}{
		{
			name:     "empty user",
			user:     "",
			expected: "",
		},
		{
			name:     "user set",
			user:     "alice",
			expected: "alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := &config.Directory{
				Path: "/some/path",
				User: tt.user,
			}
			if dir.User != tt.expected {
				t.Errorf("Directory.User = %q, want %q", dir.User, tt.expected)
			}
		})
	}
}
