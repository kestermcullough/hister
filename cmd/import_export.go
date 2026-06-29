package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/asciimoo/hister/client"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/indexer"

	"github.com/bodgit/sevenzip"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export OUTPUT_FILE [QUERY...]",
	Short: "Export indexed documents to a JSON file",
	Long: `Export all indexed documents, or only those matching a search query, to a JSON file.

Each document is written as a single JSON line. Lines not starting with '{' are
structural markers ('[', ']', ',') and can be safely skipped by parsers.

Use --start-date and --end-date (format: YYYY-MM-DD) to only export
documents added within the given date range.

Use '-' as OUTPUT_FILE to write to stdout.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		outputFile := args[0]
		queryStr := strings.Join(args[1:], " ")
		if queryStr == "" {
			queryStr = "*"
		}

		dateRange, err := parseDateRangeFlags(cmd)
		if err != nil {
			exit(1, err.Error())
		}

		var out *os.File
		if outputFile == "-" {
			out = os.Stdout
		} else {
			f, err := os.Create(outputFile)
			if err != nil {
				exit(1, "Failed to create output file: "+err.Error())
			}
			defer func() {
				if err := f.Close(); err != nil {
					log.Error().Err(err).Msg("Failed to close output file")
				}
			}()
			out = f
		}

		bw := bufio.NewWriter(out)
		defer func() {
			if err := bw.Flush(); err != nil {
				log.Error().Err(err).Msg("Failed to flush output")
			}
		}()

		if _, err := fmt.Fprintln(bw, "["); err != nil {
			exit(1, "Write error: "+err.Error())
		}

		c := newClient(client.WithTimeout(0))
		first := true
		count := 0
		pageKey := ""
		for {
			res, err := c.Search(&indexer.Query{
				Text:        queryStr,
				PageKey:     pageKey,
				IncludeHTML: true,
				IncludeText: true,
				DateFrom:    dateRange.From,
				DateTo:      dateRange.To,
			})
			if err != nil {
				exit(1, "Search failed: "+err.Error())
			}
			for _, d := range res.Documents {
				b, merr := json.Marshal(d)
				if merr != nil {
					log.Warn().Err(merr).Str("url", d.URL).Msg("Failed to serialize document, skipping")
					continue
				}
				if !first {
					if _, werr := fmt.Fprintln(bw, ","); werr != nil {
						exit(1, "Write error: "+werr.Error())
					}
				}
				first = false
				if _, werr := bw.Write(b); werr != nil {
					exit(1, "Write error: "+werr.Error())
				}
				if _, werr := fmt.Fprintln(bw); werr != nil {
					exit(1, "Write error: "+werr.Error())
				}
				count++
			}
			if res.PageKey == "" || len(res.Documents) == 0 {
				break
			}
			pageKey = res.PageKey
		}

		if _, err := fmt.Fprintln(bw, "]"); err != nil {
			exit(1, "Write error: "+err.Error())
		}

		if outputFile != "-" {
			fmt.Printf("%s Exported %d document(s) to %s\n",
				cliSuccessStyle.Render("✓"), count, cliInfoStyle.Render(outputFile))
		}
	},
}

var importCmd = &cobra.Command{
	Use:   "import INPUT_FILE_OR_DIR [INPUT_FILE_OR_DIR...]",
	Short: "Import documents from export JSON or HTML files",
	Long: `Import documents from one or more files previously created by the export
command.

JSON files are read line by line; each line starting with '{' is parsed as a
document and submitted to the running server. Content is re-processed
server-side from the stored HTML.

An input file may be a plain JSON file or a 7z-compressed archive (.7z)
containing a single JSON file.

HTML files (.html or .htm) can also be imported: the URL is extracted from
the HTML (canonical link, OpenGraph/Twitter meta tags, etc.) and the document
is submitted to the running server for processing.

Multiple files may be given; they are imported in order and the result is
reported as a combined total.

Directories may be given too; matching .json, .7z, .html, and .htm files
directly inside the directory are imported in filename order.

Use --start-date and --end-date (format: YYYY-MM-DD) to only import
documents whose "added" timestamp falls within the given date range.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		skip, _ := cmd.Flags().GetBool("skip-existing")
		global, _ := cmd.Flags().GetBool("global")

		dateRange, err := parseDateRangeFlags(cmd)
		if err != nil {
			exit(1, err.Error())
		}

		clientOpts := append([]client.Option{client.WithTimeout(0)}, targetUserIDClientOptions(cmd, global)...)
		c := newClient(clientOpts...)
		imported := 0
		skipped := 0
		errCount := 0

		inputFiles, err := expandImportInputs(args)
		if err != nil {
			exit(1, err.Error())
		}

		for _, inputFile := range inputFiles {
			var i, s, e int
			if ext := strings.ToLower(filepath.Ext(inputFile)); ext == ".html" || ext == ".htm" {
				i, s, e = importHTMLFile(c, inputFile, skip)
			} else {
				i, s, e = importJSONFile(c, inputFile, skip, dateRange.From, dateRange.To)
			}
			imported += i
			skipped += s
			errCount += e
		}

		msg := fmt.Sprintf("%s Imported %d document(s)", cliSuccessStyle.Render("✓"), imported)
		if skipped > 0 {
			msg += fmt.Sprintf(" (%d skipped)", skipped)
		}
		if errCount > 0 {
			msg += fmt.Sprintf(" (%d errors)", errCount)
		}
		fmt.Println(msg)
	},
}

func isSupportedImportInput(inputFile string) bool {
	switch strings.ToLower(filepath.Ext(inputFile)) {
	case ".json", ".7z", ".html", ".htm":
		return true
	default:
		return false
	}
}

func expandImportInputs(args []string) ([]string, error) {
	inputFiles := make([]string, 0, len(args))
	for _, input := range args {
		info, err := os.Stat(input)
		if err != nil {
			inputFiles = append(inputFiles, input)
			continue
		}
		if !info.IsDir() {
			inputFiles = append(inputFiles, input)
			continue
		}

		entries, err := os.ReadDir(input)
		if err != nil {
			return nil, fmt.Errorf("failed to read input directory %s: %w", input, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !isSupportedImportInput(entry.Name()) {
				continue
			}
			inputFiles = append(inputFiles, filepath.Join(input, entry.Name()))
		}
	}
	return inputFiles, nil
}

// importJSONFile imports documents from a JSON export file (optionally a
// 7z-compressed archive) and submits them to the running server. It returns
// the number of documents imported, skipped and failed.
func importJSONFile(c *client.Client, inputFile string, skip bool, startDate, endDate int64) (imported, skipped, errCount int) {
	var reader io.Reader

	if strings.HasSuffix(strings.ToLower(inputFile), ".7z") {
		sz, err := sevenzip.OpenReader(inputFile)
		if err != nil {
			log.Warn().Err(err).Str("file", inputFile).Msg("Failed to open 7z archive, skipping")
			return 0, 0, 1
		}
		defer func() {
			if err := sz.Close(); err != nil {
				log.Error().Err(err).Msg("Failed to close 7z archive")
			}
		}()

		var jsonEntry *sevenzip.File
		for _, entry := range sz.File {
			if strings.HasSuffix(strings.ToLower(entry.Name), ".json") {
				jsonEntry = entry
				break
			}
		}
		if jsonEntry == nil {
			log.Warn().Str("file", inputFile).Msg("No JSON file found inside 7z archive, skipping")
			return 0, 0, 1
		}
		rc, err := jsonEntry.Open()
		if err != nil {
			log.Warn().Err(err).Str("file", inputFile).Msg("Failed to open JSON entry in 7z archive, skipping")
			return 0, 0, 1
		}
		defer func() {
			if err := rc.Close(); err != nil {
				log.Error().Err(err).Msg("Failed to close 7z entry reader")
			}
		}()
		reader = rc
	} else {
		f, err := os.Open(inputFile)
		if err != nil {
			log.Warn().Err(err).Str("file", inputFile).Msg("Failed to open input file, skipping")
			return 0, 0, 1
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Error().Err(err).Msg("Failed to close input file")
			}
		}()
		reader = f
	}

	const maxLineSize = 16 * 1024 * 1024 // 16 MB covers large HTML+favicon lines
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var d document.Document
		if err := json.Unmarshal(line, &d); err != nil {
			log.Warn().Err(err).Msg("Failed to parse document line, skipping")
			errCount++
			continue
		}
		if (startDate != 0 && d.Added < startDate) || (endDate != 0 && d.Added > endDate) {
			log.Debug().Str("url", d.URL).Int64("added", d.Added).Msg("Skipping document outside of date range")
			skipped++
			continue
		}
		if skip {
			exists, err := c.DocumentExists(d.URL)
			if err != nil {
				log.Warn().Err(err).Str("url", d.URL).Msg("Failed to check if document exists, skipping")
				errCount++
				continue
			}
			if exists {
				log.Debug().Str("url", d.URL).Msg("Document already exists, skipping")
				skipped++
				continue
			}
		}
		if err := c.AddDocumentJSON(&d); err != nil {
			log.Warn().Err(err).Str("url", d.URL).Msg("Failed to add document")
			errCount++
			continue
		}
		imported++
	}

	if err := scanner.Err(); err != nil {
		log.Warn().Err(err).Str("file", inputFile).Msg("Failed to read input file")
		errCount++
	}

	return imported, skipped, errCount
}

// importHTMLFile reads a single HTML file, builds a document from it by
// extracting the URL from the HTML, and submits it to the running server. It
// returns the number of documents imported, skipped and failed.
func importHTMLFile(c *client.Client, inputFile string, skip bool) (imported, skipped, errCount int) {
	data, err := os.ReadFile(inputFile)
	if err != nil {
		log.Warn().Err(err).Str("file", inputFile).Msg("Failed to read HTML file, skipping")
		return 0, 0, 1
	}

	d, err := document.FromHTML(string(data))
	if err != nil {
		log.Warn().Err(err).Str("file", inputFile).Msg("Failed to import HTML file, skipping")
		return 0, 0, 1
	}

	if skip {
		exists, err := c.DocumentExists(d.URL)
		if err != nil {
			log.Warn().Err(err).Str("url", d.URL).Msg("Failed to check if document exists, skipping")
			return 0, 0, 1
		}
		if exists {
			log.Debug().Str("url", d.URL).Msg("Document already exists, skipping")
			return 0, 1, 0
		}
	}

	if err := c.AddDocumentJSON(d); err != nil {
		log.Warn().Err(err).Str("url", d.URL).Msg("Failed to add document")
		return 0, 0, 1
	}

	return 1, 0, 0
}
