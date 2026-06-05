package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/asciimoo/hister/server/document"
)

func (c *Client) AddDocumentJSON(doc *document.Document) (err error) {
	if c.allowSensitive {
		doc.SkipSensitiveCheck = true
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	req, err := c.newRequest("POST", "/api/add", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp, &err)
	return checkStatus(resp)
}

func (c *Client) AddPage(u, title, text string) (err error) {
	formData := url.Values{"url": {u}, "title": {title}, "text": {text}}
	req, err := c.newRequest("POST", "/api/add", strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp, &err)
	return checkStatus(resp)
}

func (c *Client) DocumentExists(u string) (_ bool, err error) {
	req, err := c.newRequest("HEAD", "/api/document?url="+url.QueryEscape(u), nil)
	if err != nil {
		return false, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer closeBody(resp, &err)
	return resp.StatusCode == http.StatusOK, nil
}

func (c *Client) Reindex(skipSensitive, detectLanguages bool) (err error) {
	type reindexRequest struct {
		SkipSensitive   bool `json:"skipSensitive"`
		DetectLanguages bool `json:"detectLanguages"`
	}
	data, err := json.Marshal(reindexRequest{SkipSensitive: skipSensitive, DetectLanguages: detectLanguages})
	if err != nil {
		return err
	}
	req, err := c.newRequest("POST", "/api/reindex", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp, &err)
	return checkStatus(resp)
}

func (c *Client) DeleteDocument(u string) (err error) {
	return c.DeleteDocuments("url:" + u)
}

func (c *Client) DeleteDocuments(query string) (err error) {
	data, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return err
	}
	req, err := c.newRequest("POST", "/api/delete", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp, &err)
	return checkStatus(resp)
}

// CleanupResult holds the number of orphaned files removed by the cleanup endpoint.
type CleanupResult struct {
	HTMLRemoved    int `json:"htmlRemoved"`
	FaviconRemoved int `json:"faviconRemoved"`
}

func (c *Client) Cleanup() (result CleanupResult, err error) {
	req, err := c.newRequest("POST", "/api/cleanup", nil)
	if err != nil {
		return result, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return result, err
	}
	defer closeBody(resp, &err)
	if err = checkStatus(resp); err != nil {
		return result, err
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}
