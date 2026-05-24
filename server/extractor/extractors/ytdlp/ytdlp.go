// Package ytdlp provides an extractor for video pages using the yt-dlp tool.
package ytdlp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/types"
)

const (
	cacheTTL     = 10 * time.Minute
	maxCacheSize = 500
	maxThumbSize = 5 << 20 // 5 MB
)

// YtdlpExtractor extracts video metadata using yt-dlp.
type YtdlpExtractor struct {
	cfg   *config.Extractor
	cache sync.Map // URL -> *cachedInfo
}

func (e *YtdlpExtractor) Name() string {
	return "Ytdlp"
}

// Description returns a short summary of what this extractor does.
func (e *YtdlpExtractor) Description() string {
	return "Extracts video metadata (title, description, chapters, subtitles, thumbnail) from video hosting sites using the yt-dlp tool."
}

func (e *YtdlpExtractor) GetConfig() *config.Extractor {
	if e.cfg == nil {
		return &config.Extractor{
			Enable: false,
			Options: map[string]any{
				"binary":          "yt-dlp",
				"timeout":         15,
				"fetch_subtitles": false,
				"sub_language":    "auto",
			},
		}
	}
	return e.cfg
}

func (e *YtdlpExtractor) SetConfig(c *config.Extractor) error {
	for k := range c.Options {
		switch k {
		case "binary", "timeout", "fetch_subtitles", "sub_language",
			"cookies_file", "cookies_from_browser", "extra_args":
		default:
			return fmt.Errorf("unknown option %q", k)
		}
	}
	e.cfg = c
	return nil
}

func (e *YtdlpExtractor) binary() string {
	if b, ok := e.GetConfig().Options["binary"].(string); ok && b != "" {
		return b
	}
	return "yt-dlp"
}

func (e *YtdlpExtractor) timeout() time.Duration {
	cfg := e.GetConfig()
	switch v := cfg.Options["timeout"].(type) {
	case int:
		return time.Duration(v) * time.Second
	case float64:
		return time.Duration(v) * time.Second
	}
	return 15 * time.Second
}

func (e *YtdlpExtractor) fetchSubtitlesEnabled() bool {
	v, ok := e.GetConfig().Options["fetch_subtitles"].(bool)
	return ok && v
}

func (e *YtdlpExtractor) subLanguage() string {
	if s, ok := e.GetConfig().Options["sub_language"].(string); ok && s != "" {
		return s
	}
	return "en"
}

// cookieArgs returns yt-dlp CLI flags for cookie authentication based on config.
func (e *YtdlpExtractor) cookieArgs() []string {
	var args []string
	if f, ok := e.GetConfig().Options["cookies_file"].(string); ok && f != "" {
		args = append(args, "--cookies", f)
	}
	if b, ok := e.GetConfig().Options["cookies_from_browser"].(string); ok && b != "" {
		args = append(args, "--cookies-from-browser", b)
	}
	return args
}

func (e *YtdlpExtractor) Match(d *document.Document) bool {
	u, err := url.Parse(d.URL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	matched := false
	for _, domain := range knownDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			matched = true
			break
		}
	}
	if !matched {
		for _, sub := range knownHostSubstrings {
			if strings.Contains(host, sub) {
				matched = true
				break
			}
		}
	}
	if !matched {
		return false
	}
	// Reject bare homepages — yt-dlp cannot extract anything from a site root
	// with no path or query parameters (e.g. "https://www.youtube.com/").
	path := strings.TrimRight(u.Path, "/")
	if path == "" && u.RawQuery == "" {
		return false
	}
	return true
}

// getInfo returns cached videoInfo or fetches it via yt-dlp.
func (e *YtdlpExtractor) getInfo(videoURL string) (*videoInfo, error) {
	if cached, ok := e.cache.Load(videoURL); ok {
		ci := cached.(*cachedInfo)
		if time.Since(ci.fetchedAt) < cacheTTL {
			return ci.info, nil
		}
		e.cache.Delete(videoURL)
	}

	info, err := e.fetchInfo(videoURL)
	if err != nil {
		return nil, err
	}

	e.pruneCache()
	e.cache.Store(videoURL, &cachedInfo{info: info, fetchedAt: time.Now()})
	return info, nil
}

// pruneCache evicts expired entries and, if still over maxCacheSize, evicts the oldest.
func (e *YtdlpExtractor) pruneCache() {
	var count int
	var oldestKey string
	var oldestTime time.Time

	e.cache.Range(func(key, value any) bool {
		ci := value.(*cachedInfo)
		if time.Since(ci.fetchedAt) >= cacheTTL {
			e.cache.Delete(key)
			return true
		}
		count++
		if oldestKey == "" || ci.fetchedAt.Before(oldestTime) {
			oldestKey = key.(string)
			oldestTime = ci.fetchedAt
		}
		return true
	})

	if count >= maxCacheSize && oldestKey != "" {
		e.cache.Delete(oldestKey)
	}
}

func (e *YtdlpExtractor) fetchInfo(videoURL string) (*videoInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout())
	defer cancel()

	args := []string{
		"--dump-json",
		"--no-download",
		"--no-playlist",
		"--no-warnings",
	}
	args = append(args, e.cookieArgs()...)
	if extra, ok := e.GetConfig().Options["extra_args"].([]any); ok {
		for _, a := range extra {
			if s, ok := a.(string); ok {
				args = append(args, s)
			}
		}
	}
	args = append(args, videoURL)

	// #nosec G204 -- binary path and args are admin-configured, not user input.
	cmd := exec.CommandContext(ctx, e.binary(), args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	log.Debug().Str("args", strings.Join(cmd.Args, " ")).Msg("yt-dlp executing")
	out, err := cmd.Output()
	log.Debug().Str("stdout", strings.TrimSpace(string(out))).Str("stderr", strings.TrimSpace(stderr.String())).Msg("yt-dlp output")
	if err != nil {
		if s := stderr.String(); s != "" {
			return nil, fmt.Errorf("yt-dlp failed: %w: %s", err, strings.TrimSpace(s))
		}
		return nil, fmt.Errorf("yt-dlp failed: %w", err)
	}
	var info videoInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("failed to parse yt-dlp output: %w", err)
	}
	return &info, nil
}

// downloadThumbnail fetches the thumbnail image and returns it as a base64 data URI.
func (e *YtdlpExtractor) downloadThumbnail(thumbnailURL string) string {
	if thumbnailURL == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, thumbnailURL, nil)
	if err != nil {
		return ""
	}
	cli := &http.Client{Timeout: e.timeout()}
	resp, err := cli.Do(req)
	if err != nil {
		return ""
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Warn().Err(cerr).Msg("failed to close thumbnail response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxThumbSize))
	if err != nil {
		return ""
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	return fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(data))
}

func (e *YtdlpExtractor) Extract(d *document.Document) (types.ExtractorState, error) {
	info, err := e.getInfo(d.URL)
	if err != nil {
		return types.ExtractorContinue, err
	}

	if info.Title != "" {
		d.Title = info.Title
	}

	var text strings.Builder
	if info.Description != "" {
		text.WriteString(info.Description)
	}
	if info.Uploader != "" {
		text.WriteString("\n\nUploader: ")
		text.WriteString(info.Uploader)
	}
	if len(info.Tags) > 0 {
		text.WriteString("\nTags: ")
		text.WriteString(strings.Join(info.Tags, ", "))
	}
	if len(info.Categories) > 0 {
		text.WriteString("\nCategories: ")
		text.WriteString(strings.Join(info.Categories, ", "))
	}
	if len(info.Chapters) > 0 {
		text.WriteString("\n\nChapters:\n")
		for _, ch := range info.Chapters {
			fmt.Fprintf(&text, "  %s %s\n", formatDuration(ch.StartTime), ch.Title)
		}
	}
	if info.PlaylistTitle != "" {
		text.WriteString("\nPlaylist: ")
		text.WriteString(info.PlaylistTitle)
		if info.PlaylistIndex != nil && info.PlaylistCount != nil {
			fmt.Fprintf(&text, " (%d/%d)", *info.PlaylistIndex, *info.PlaylistCount)
		}
	}

	if e.fetchSubtitlesEnabled() {
		if transcript := e.fetchSubtitleText(info); transcript != "" {
			text.WriteString("\n\nTranscript:\n")
			text.WriteString(transcript)
		}
	}

	d.Text = strings.TrimSpace(text.String())

	// Store structured metadata.
	if d.Metadata == nil {
		d.Metadata = make(map[string]any)
	}
	if thumb := e.downloadThumbnail(info.Thumbnail); thumb != "" {
		d.Metadata["thumbnail"] = thumb
	}
	if info.Duration > 0 {
		d.Metadata["duration"] = info.Duration
	}
	if info.UploadDate != "" {
		d.Metadata["upload_date"] = formatDate(info.UploadDate)
	}
	if info.Uploader != "" {
		d.Metadata["uploader"] = info.Uploader
	}
	if info.ViewCount > 0 {
		d.Metadata["view_count"] = info.ViewCount
	}

	return types.ExtractorStop, nil
}

func (e *YtdlpExtractor) Preview(d *document.Document) (types.PreviewResponse, types.ExtractorState, error) {
	info, err := e.getInfo(d.URL)
	if err != nil {
		return types.PreviewResponse{}, types.ExtractorContinue, err
	}

	// Use cached thumbnail from metadata, or download it.
	thumbDataURI := ""
	if d.Metadata != nil {
		if t, ok := d.Metadata["thumbnail"].(string); ok {
			thumbDataURI = t
		}
	}
	if thumbDataURI == "" {
		thumbDataURI = e.downloadThumbnail(info.Thumbnail)
	}

	// Build structured preview data for the frontend.
	preview := videoPreviewData{
		Title:             info.Title,
		Uploader:          info.Uploader,
		Duration:          info.Duration,
		DurationFormatted: formatDuration(info.Duration),
		UploadDate:        formatDate(info.UploadDate),
		ViewCount:         info.ViewCount,
		LikeCount:         info.LikeCount,
		Categories:        info.Categories,
		Tags:              info.Tags,
		Description:       info.Description,
		Thumbnail:         thumbDataURI,
		WebpageURL:        info.WebpageURL,
	}

	if len(info.Chapters) > 0 {
		preview.Chapters = make([]chapterPreview, len(info.Chapters))
		for i, ch := range info.Chapters {
			preview.Chapters[i] = chapterPreview{
				Title:     ch.Title,
				StartTime: formatDuration(ch.StartTime),
			}
		}
	}

	if info.PlaylistTitle != "" {
		preview.Playlist = &playlistPreview{
			Title: info.PlaylistTitle,
		}
		if info.PlaylistIndex != nil {
			preview.Playlist.Index = *info.PlaylistIndex
		}
		if info.PlaylistCount != nil {
			preview.Playlist.Count = *info.PlaylistCount
		}
	}

	if e.fetchSubtitlesEnabled() {
		preview.Transcript = e.fetchSubtitleText(info)
	}

	data, err := json.Marshal(preview)
	if err != nil {
		return types.PreviewResponse{}, types.ExtractorContinue, err
	}

	return types.PreviewResponse{
		Content:  string(data),
		Template: "video",
	}, types.ExtractorStop, nil
}
