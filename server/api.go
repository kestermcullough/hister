package server

import (
	"fmt"
)

const (
	// GET is HTTP GET request type
	GET string = "GET"
	// POST is HTTP POST request type
	POST string = "POST"
	// PUT is HTTP PUT request type
	PUT string = "PUT"
	// PATCH is HTTP PATCH request type
	PATCH string = "PATCH"
	// HEAD is HTTP HEAD request type
	HEAD string = "HEAD"
)

type endpointHandler func(*webContext)

// EndpointArg represents a query-string or form parameter for an API endpoint.
type EndpointArg struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// JSONSchemaField represents a single field in a JSON request body schema.
// Fields may be nested to describe object and array element types.
type JSONSchemaField struct {
	Name        string             `json:"name"`
	Type        string             `json:"type"`
	Required    bool               `json:"required"`
	Description string             `json:"description"`
	Fields      []*JSONSchemaField `json:"fields,omitempty"`
}

// Endpoint represents an API endpoint definition.
type Endpoint struct {
	Name         string
	Path         string
	Method       string
	CSRFRequired bool
	NoAuth       bool
	AdminOnly    bool
	Handler      endpointHandler `json:"-"`
	Description  string
	Args         []*EndpointArg
	// JSONSchema describes the fields of the JSON request body, if the endpoint
	// consumes application/json instead of (or in addition to) form data.
	JSONSchema []*JSONSchemaField
}

func (e *Endpoint) Pattern() string {
	return fmt.Sprintf("%s %s", e.Method, e.Path)
}

// Endpoints contains all registered API endpoints.
var Endpoints []*Endpoint

func init() {
	Endpoints = []*Endpoint{
		{
			Name:         "Config",
			Path:         "/api/config",
			Method:       GET,
			CSRFRequired: true,
			NoAuth:       true,
			Handler:      serveConfig,
			Description:  "Return server configuration (base URL, hotkeys, auth mode, CSRF token, etc.)",
		},
		{
			Name:        "Search",
			Path:        "/search",
			Method:      GET,
			Handler:     serveSearch,
			Description: "Search endpoint. With a query parameter it returns JSON results directly. Without one it upgrades to a WebSocket connection that accepts repeated JSON Query messages and streams back results.",
			Args: []*EndpointArg{
				{Name: "q", Type: "string", Required: false, Description: "Plain-text search query"},
				{Name: "query", Type: "string (JSON)", Required: false, Description: "JSON-encoded Query object; used as an alternative to individual parameters"},
				{Name: "date_from", Type: "string (YYYY-MM-DD)", Required: false, Description: "Return only results indexed on or after this date"},
				{Name: "date_to", Type: "string (YYYY-MM-DD)", Required: false, Description: "Return only results indexed on or before this date"},
				{Name: "include_html", Type: "string (0 or 1)", Required: false, Description: "Include raw HTML in results"},
				{Name: "page_key", Type: "string", Required: false, Description: "Pagination cursor returned by a previous response"},
				{Name: "sort", Type: "string", Required: false, Description: "Sort order (e.g. \"date\")"},
				{Name: "semantic", Type: "string (0/1/true/false)", Required: false, Description: "Enable semantic search"},
				{Name: "semantic_threshold", Type: "float", Required: false, Description: "Minimum similarity score for semantic results"},
			},
			JSONSchema: []*JSONSchemaField{
				{Name: "text", Type: "string", Required: false, Description: "Search query string"},
				{Name: "date_from", Type: "int64", Required: false, Description: "Unix timestamp lower bound for indexed date"},
				{Name: "date_to", Type: "int64", Required: false, Description: "Unix timestamp upper bound for indexed date"},
				{Name: "include_html", Type: "bool", Required: false, Description: "Include raw HTML in results"},
				{Name: "page_key", Type: "string", Required: false, Description: "Pagination cursor from a previous response"},
				{Name: "sort", Type: "string", Required: false, Description: "Sort order (e.g. \"date\")"},
				{Name: "limit", Type: "int", Required: false, Description: "Maximum number of results"},
				{Name: "highlight", Type: "string", Required: false, Description: "Field to highlight matched terms in"},
				{Name: "semantic_enabled", Type: "bool", Required: false, Description: "Enable semantic search"},
				{Name: "semantic_threshold", Type: "float64", Required: false, Description: "Minimum similarity score for semantic results"},
				{Name: "semantic_weight", Type: "float64", Required: false, Description: "Weight applied to semantic vs full-text scores"},
				{Name: "facets", Type: "bool", Required: false, Description: "Include facet counts (domain, language) in the response"},
				{Name: "facet_term_size", Type: "int", Required: false, Description: "Override the default top-N cap for term facets"},
			},
		},
		{
			Name:        "Suggest",
			Path:        "/suggest",
			Method:      GET,
			Handler:     serveSuggest,
			Description: "OpenSearch suggestions endpoint; returns query completions",
			Args: []*EndpointArg{
				{Name: "q", Type: "string", Required: true, Description: "Partial query string to complete"},
			},
		},
		// tmp added for backward compatibility
		{
			Name:         "Add",
			Path:         "/api/add",
			Method:       GET,
			CSRFRequired: true,
			Handler:      serveAdd,
			Description:  "Add document form (returns 200; kept for backward compatibility)",
		},
		{
			Name:         "Add",
			Path:         "/api/add",
			Method:       POST,
			CSRFRequired: true,
			Handler:      serveAdd,
			Description:  "Index a document. Accepts either application/x-www-form-urlencoded or application/json.",
			Args: []*EndpointArg{
				{Name: "url", Type: "string", Required: true, Description: "URL of the document to index"},
				{Name: "title", Type: "string", Required: false, Description: "Document title"},
				{Name: "text", Type: "string", Required: false, Description: "Plain-text content"},
			},
			JSONSchema: []*JSONSchemaField{
				{Name: "url", Type: "string", Required: true, Description: "URL of the document to index"},
				{Name: "title", Type: "string", Required: false, Description: "Document title"},
				{Name: "text", Type: "string", Required: false, Description: "Plain-text content (overrides server-side HTML extraction)"},
				{Name: "html", Type: "string", Required: false, Description: "Raw HTML source (text is extracted server-side)"},
				{Name: "favicon", Type: "string", Required: false, Description: "Base64-encoded favicon data URI"},
				{Name: "label", Type: "string", Required: false, Description: "User-defined label for the document"},
			},
		},
		// alias for /api/add - backward compatibility - use /api/add in the future
		{
			Name:         "Add (legacy path)",
			Path:         "/add",
			Method:       POST,
			CSRFRequired: true,
			Handler:      serveAdd,
			Description:  "Index a document (legacy path; prefer /api/add)",
			Args: []*EndpointArg{
				{Name: "url", Type: "string", Required: true, Description: "URL of the document to index"},
				{Name: "title", Type: "string", Required: false, Description: "Document title"},
				{Name: "text", Type: "string", Required: false, Description: "Plain-text content"},
			},
			JSONSchema: []*JSONSchemaField{
				{Name: "url", Type: "string", Required: true, Description: "URL of the document to index"},
				{Name: "title", Type: "string", Required: false, Description: "Document title"},
				{Name: "text", Type: "string", Required: false, Description: "Plain-text content (overrides server-side HTML extraction)"},
				{Name: "html", Type: "string", Required: false, Description: "Raw HTML source (text is extracted server-side)"},
				{Name: "favicon", Type: "string", Required: false, Description: "Base64-encoded favicon data URI"},
			},
		},
		{
			Name:         "Add PDF",
			Path:         "/api/add_pdf",
			Method:       POST,
			CSRFRequired: true,
			Handler:      serveAddPDF,
			Description:  "Index a PDF document. Accepts application/json with a document object and base64-encoded PDF content.",
			JSONSchema: []*JSONSchemaField{
				{
					Name:        "document",
					Type:        "object",
					Required:    true,
					Description: "Document metadata",
					Fields: []*JSONSchemaField{
						{Name: "url", Type: "string", Required: true, Description: "URL or file:// URI identifying the PDF"},
						{Name: "title", Type: "string", Required: false, Description: "Document title"},
						{Name: "label", Type: "string", Required: false, Description: "User-defined label"},
					},
				},
				{Name: "pdf", Type: "string", Required: true, Description: "Base64-encoded PDF binary content"},
			},
		},
		{
			Name:         "Get document",
			Path:         "/api/document",
			Method:       GET,
			CSRFRequired: false,
			Handler:      serveGet,
			Description:  "Retrieve a stored document by its URL",
			Args: []*EndpointArg{
				{Name: "url", Type: "string", Required: true, Description: "URL of the document"},
			},
		},
		{
			Name:         "Rules",
			Path:         "/api/rules",
			Method:       GET,
			CSRFRequired: true,
			Handler:      serveRules,
			Description:  "Retrieve current skip/priority rules and query aliases",
		},
		{
			Name:         "Save rules",
			Path:         "/api/rules",
			Method:       POST,
			CSRFRequired: true,
			Handler:      serveRules,
			Description:  "Update skip/priority rules. Accepts application/x-www-form-urlencoded.",
			Args: []*EndpointArg{
				{Name: "skip", Type: "string", Required: false, Description: "Space-separated list of URL regex patterns to skip during indexing"},
				{Name: "priority", Type: "string", Required: false, Description: "Space-separated list of URL regex patterns to surface first in results"},
			},
		},
		{
			Name:         "History",
			Path:         "/api/history",
			Method:       GET,
			CSRFRequired: true,
			Handler:      serveHistory,
			Description:  "Retrieve recently indexed documents or search query history",
			Args: []*EndpointArg{
				{Name: "opened", Type: "bool", Required: false, Description: "When \"true\", returns search query history instead of recently indexed documents"},
				{Name: "last_id", Type: "uint", Required: false, Description: "Pagination cursor: last history item ID from a previous response (used with opened=true)"},
				{Name: "last", Type: "string", Required: false, Description: "Pagination cursor: URL of the last indexed document from a previous response"},
			},
		},
		{
			Name:         "Add history item",
			Path:         "/api/history",
			Method:       POST,
			CSRFRequired: true,
			Handler:      serveSaveHistory,
			Description:  "Record or delete a search query history entry",
			JSONSchema: []*JSONSchemaField{
				{Name: "url", Type: "string", Required: false, Description: "URL of the visited page"},
				{Name: "title", Type: "string", Required: false, Description: "Page title"},
				{Name: "query", Type: "string", Required: false, Description: "Search query string that led to this page"},
				{Name: "delete", Type: "bool", Required: false, Description: "When true, removes the matching history entry instead of adding one"},
			},
		},
		{
			Name:         "Delete",
			Path:         "/api/delete",
			Method:       POST,
			CSRFRequired: true,
			Handler:      serveDelete,
			Description:  "Delete documents matching a search query. Non-admin users are restricted to their own documents.",
			JSONSchema: []*JSONSchemaField{
				{Name: "query", Type: "string", Required: true, Description: "Search query string selecting documents to delete (same syntax as the search endpoint)"},
			},
		},
		{
			Name:         "Delete alias",
			Path:         "/api/delete_alias",
			Method:       POST,
			CSRFRequired: true,
			Handler:      serveDeleteAlias,
			Description:  "Remove a query alias",
			Args: []*EndpointArg{
				{Name: "alias", Type: "string", Required: true, Description: "Alias keyword to remove"},
			},
		},
		{
			Name:         "Add alias",
			Path:         "/api/add_alias",
			Method:       POST,
			CSRFRequired: true,
			Handler:      serveAddAlias,
			Description:  "Add or update a query alias",
			Args: []*EndpointArg{
				{Name: "alias-keyword", Type: "string", Required: true, Description: "Shorthand keyword the user types in search"},
				{Name: "alias-value", Type: "string", Required: true, Description: "Expanded query expression that the keyword maps to"},
			},
		},
		{
			Name:         "Preview",
			Path:         "/api/preview",
			Method:       GET,
			CSRFRequired: false,
			Handler:      servePreview,
			Description:  "Render a readable preview of a stored document",
			Args: []*EndpointArg{
				{Name: "url", Type: "string", Required: true, Description: "URL of the document to preview"},
			},
		},
		{
			Name:         "Extractors",
			Path:         "/api/extractors",
			Method:       GET,
			CSRFRequired: false,
			NoAuth:       true,
			Handler:      serveExtractors,
			Description:  "List all registered extractors with their name, description, and enabled state",
		},
		{
			Name:         "Stats",
			Path:         "/api/stats",
			Method:       GET,
			CSRFRequired: false,
			Handler:      serveStats,
			Description:  "Return index statistics (document count, rule count, recent searches)",
		},
		{
			Name:         "File",
			Path:         "/api/file",
			Method:       GET,
			CSRFRequired: false,
			Handler:      serveFile,
			Description:  "Serve the raw content of a locally indexed file",
			Args: []*EndpointArg{
				{Name: "path", Type: "string", Required: true, Description: "Absolute path to the file"},
			},
		},
		{
			Name:         "Batch",
			Path:         "/api/batch",
			Method:       POST,
			CSRFRequired: true,
			Handler:      serveBatch,
			Description:  "Execute up to 100 add/delete/get operations in a single request (5 MB body limit)",
			JSONSchema: []*JSONSchemaField{
				{
					Name:        "ops",
					Type:        "array",
					Required:    true,
					Description: "List of operations to execute (maximum 100)",
					Fields: []*JSONSchemaField{
						{Name: "op", Type: "string", Required: true, Description: "Operation type: \"add\", \"delete\", or \"get\""},
						{Name: "url", Type: "string", Required: true, Description: "Document URL"},
						{Name: "title", Type: "string", Required: false, Description: "Document title (add only)"},
						{Name: "text", Type: "string", Required: false, Description: "Plain-text content (add only)"},
						{Name: "html", Type: "string", Required: false, Description: "Raw HTML source (add only; text extracted server-side)"},
						{Name: "favicon", Type: "string", Required: false, Description: "Base64-encoded favicon data URI (add only)"},
					},
				},
			},
		},
		{
			Name:         "Reindex",
			Path:         "/api/reindex",
			Method:       POST,
			CSRFRequired: true,
			AdminOnly:    true,
			Handler:      serveReindex,
			Description:  "Rebuild the search index from all stored documents (admin only)",
			JSONSchema: []*JSONSchemaField{
				{Name: "skipSensitive", Type: "bool", Required: false, Description: "Skip documents that match sensitive-content patterns"},
				{Name: "detectLanguages", Type: "bool", Required: false, Description: "Enable per-language index routing during reindex"},
			},
		},
		{
			Name:         "API",
			Path:         "/api",
			Method:       GET,
			CSRFRequired: false,
			Handler:      serveAPI,
			Description:  "Return this API documentation as JSON",
		},
		{
			Name:         "Login",
			Path:         "/api/login",
			Method:       POST,
			CSRFRequired: true,
			NoAuth:       true,
			Handler:      serveLogin,
			Description:  "Authenticate with username and password and create a session",
			JSONSchema: []*JSONSchemaField{
				{Name: "username", Type: "string", Required: true, Description: "Account username"},
				{Name: "password", Type: "string", Required: true, Description: "Account password"},
			},
		},
		{
			Name:         "Logout",
			Path:         "/api/logout",
			Method:       POST,
			CSRFRequired: true,
			Handler:      serveLogout,
			Description:  "Destroy the current session",
		},
		{
			Name:         "Profile",
			Path:         "/api/profile",
			Method:       GET,
			CSRFRequired: false,
			Handler:      serveProfile,
			Description:  "Return the authenticated user's profile information",
		},
		{
			Name:         "GenerateToken",
			Path:         "/api/profile/token",
			Method:       POST,
			CSRFRequired: true,
			Handler:      serveGenerateToken,
			Description:  "Regenerate the API access token for the current user",
		},
		{
			Name:        "MCP",
			Path:        "/mcp",
			Method:      POST,
			Handler:     serveMCP,
			Description: "Model Context Protocol endpoint (JSON-RPC 2.0 / Streamable HTTP). Exposes the search tool to AI assistants.",
			JSONSchema: []*JSONSchemaField{
				{Name: "jsonrpc", Type: "string", Required: true, Description: "JSON-RPC version; must be \"2.0\""},
				{Name: "id", Type: "string | number | null", Required: false, Description: "Request identifier; omit for notifications"},
				{Name: "method", Type: "string", Required: true, Description: "RPC method name (e.g. \"tools/call\", \"initialize\", \"tools/list\")"},
				{
					Name:        "params",
					Type:        "object",
					Required:    false,
					Description: "Method parameters; shape depends on the method",
					Fields: []*JSONSchemaField{
						{Name: "name", Type: "string", Required: true, Description: "Tool name; must be \"search\" for tools/call"},
						{
							Name:        "arguments",
							Type:        "object",
							Required:    false,
							Description: "Tool arguments for tools/call",
							Fields: []*JSONSchemaField{
								{Name: "query", Type: "string", Required: true, Description: "Search query string"},
								{Name: "limit", Type: "int", Required: false, Description: "Maximum number of results (default 10)"},
							},
						},
					},
				},
			},
		},
		{
			Name:        "OAuthRedirect",
			Path:        "/api/oauth",
			Method:      GET,
			NoAuth:      true,
			Handler:     serveOAuthRedirect,
			Description: "Start OAuth authentication flow for a given provider",
			Args: []*EndpointArg{
				{Name: "provider", Type: "string", Required: true, Description: "OAuth provider name (github, google, oidc)"},
			},
		},
		{
			Name:        "OAuthCallback",
			Path:        "/api/oauth/callback",
			Method:      GET,
			NoAuth:      true,
			Handler:     serveOAuthCallback,
			Description: "OAuth provider callback handler",
			Args: []*EndpointArg{
				{Name: "provider", Type: "string", Required: true, Description: "OAuth provider name"},
				{Name: "code", Type: "string", Required: true, Description: "Authorization code returned by the provider"},
				{Name: "state", Type: "string", Required: true, Description: "CSRF state token issued during the redirect step"},
			},
		},
	}
}
