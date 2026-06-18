// SPDX-License-Identifier: AGPL-3.0-or-later

package vectorstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/asciimoo/hister/config"
)

// Embedder calls an OpenAI-compatible /v1/embeddings endpoint to convert text
// into float32 vectors. It also handles text chunking for long documents.
type Embedder struct {
	endpoint         string
	model            string
	apiKey           string
	headers          map[string]string
	dimensions       int
	client           *http.Client
	maxContextLength int
	chunkOverlap     int
	queryPrefix      string
	documentPrefix   string
	sem              chan struct{} // nil means unlimited concurrency
}

const embeddingMaxAttempts = 3

// NewEmbedder creates an Embedder from the semantic search config.
func NewEmbedder(cfg *config.SemanticSearch) *Embedder {
	var sem chan struct{}
	if cfg.MaxEmbeddingConcurrency > 0 {
		sem = make(chan struct{}, cfg.MaxEmbeddingConcurrency)
	}
	return &Embedder{
		endpoint:         cfg.EmbeddingEndpoint,
		model:            cfg.EmbeddingModel,
		apiKey:           cfg.APIKey,
		headers:          cfg.Headers,
		dimensions:       cfg.Dimensions,
		maxContextLength: cfg.MaxContextLength,
		chunkOverlap:     cfg.ChunkOverlap,
		queryPrefix:      cfg.QueryPrefix,
		documentPrefix:   cfg.DocumentPrefix,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		sem: sem,
	}
}

type embeddingRequest struct {
	Model string `json:"model"`
	Input any    `json:"input"` // string for single, []string for batch
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

type embeddingStatusError struct {
	statusCode int
	body       string
}

func (e *embeddingStatusError) Error() string {
	return fmt.Sprintf("embedding endpoint returned %d: %s", e.statusCode, e.body)
}

func (e *embeddingStatusError) transient() bool {
	switch e.statusCode {
	case http.StatusRequestTimeout,
		http.StatusTooEarly,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func embeddingRetryDelay(attempt int) time.Duration {
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func shouldRetryEmbeddingError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}

	var statusErr *embeddingStatusError
	if errors.As(err, &statusErr) {
		return statusErr.transient()
	}

	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

// doEmbeddingRequestOnce sends one embedding request to the endpoint and returns
// the parsed response. input is either a string (single) or []string (batch).
func (e *Embedder) doEmbeddingRequestOnce(ctx context.Context, input any) (_ *embeddingResponse, err error) {
	body, err := json.Marshal(embeddingRequest{
		Model: e.model,
		Input: input,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}
	for k, v := range e.headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); err == nil {
			err = cerr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &embeddingStatusError{statusCode: resp.StatusCode, body: string(respBody)}
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	return &result, nil
}

// doEmbeddingRequest sends an embedding request, retrying transient endpoint or
// network failures while respecting the caller's context.
func (e *Embedder) doEmbeddingRequest(ctx context.Context, input any) (*embeddingResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if e.sem != nil {
		select {
		case e.sem <- struct{}{}:
			defer func() { <-e.sem }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	var err error
	for attempt := range embeddingMaxAttempts {
		var result *embeddingResponse
		result, err = e.doEmbeddingRequestOnce(ctx, input)
		if err == nil {
			return result, nil
		}
		if attempt == embeddingMaxAttempts-1 || !shouldRetryEmbeddingError(ctx, err) {
			return nil, err
		}

		timer := time.NewTimer(embeddingRetryDelay(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, err
}

// Embed converts a single text into a float32 vector.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	result, err := e.doEmbeddingRequest(ctx, text)
	if err != nil {
		return nil, err
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embedding response contained no data")
	}
	if got := len(result.Data[0].Embedding); e.dimensions > 0 && got != e.dimensions {
		return nil, fmt.Errorf("embedding dimension mismatch: expected %d, got %d", e.dimensions, got)
	}
	return toFloat32(result.Data[0].Embedding), nil
}

// EmbedQuery embeds a search query, prepending the configured query prefix
// (e.g. "search_query: ") when set. Many embedding models (BGE, E5, Nomic,
// GTE) produce better recall when queries and documents use distinct prefixes.
func (e *Embedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return e.Embed(ctx, e.queryPrefix+text)
}

// EmbedBatch converts multiple texts in a single request.
func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result, err := e.doEmbeddingRequest(ctx, texts)
	if err != nil {
		return nil, err
	}
	vectors := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		if got := len(d.Embedding); e.dimensions > 0 && got != e.dimensions {
			return nil, fmt.Errorf("embedding dimension mismatch at index %d: expected %d, got %d", i, e.dimensions, got)
		}
		vectors[i] = toFloat32(d.Embedding)
	}
	return vectors, nil
}

func toFloat32(f64 []float64) []float32 {
	f32 := make([]float32, len(f64))
	for i, v := range f64 {
		f32[i] = float32(v)
	}
	return f32
}

// ChunkAndEmbed splits text into overlapping chunks, prepends document context
// metadata title and the configured document prefix to each chunk,
// batch-embeds them, and returns Chunk values ready for storage. Returns nil
// (not an error) when the text is empty.
func (e *Embedder) ChunkAndEmbed(ctx context.Context, text, title string) ([]Chunk, error) {
	textChunks := ChunkText(text, e.maxContextLength, e.chunkOverlap)
	if len(textChunks) == 0 {
		return nil, nil
	}

	header := e.documentPrefix + "Title: " + title + " | "

	texts := make([]string, len(textChunks))
	for i, tc := range textChunks {
		texts[i] = header + tc.Text
	}

	vectors, err := e.EmbedBatch(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(textChunks) {
		return nil, fmt.Errorf("embedding count mismatch: expected %d, got %d", len(textChunks), len(vectors))
	}

	chunks := make([]Chunk, len(textChunks))
	for i := range textChunks {
		chunks[i] = Chunk{
			Index:     i,
			Text:      textChunks[i].Text,
			Embedding: vectors[i],
		}
	}
	return chunks, nil
}
