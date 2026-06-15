package server

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	iofs "io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/files"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/extractor"
	"github.com/asciimoo/hister/server/indexer"
	"github.com/asciimoo/hister/server/model"
	"github.com/asciimoo/hister/server/static"
	"github.com/asciimoo/hister/server/types"

	"github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// Version is set by the main package before Listen() is called so that MCP
// and other server responses can expose the running binary version.
var Version = "unknown"

var (
	appSubFS         iofs.FS
	staticFileServer http.Handler
	sessionStore     *sessions.CookieStore
	errCSRFMismatch  = errors.New("CSRF token mismatch")
	storeName        = "hister"
	tokName          = "csrf_token"
	staticTextFiles  map[string][]byte
)

type historyItem struct {
	URL    string `json:"url"`
	Title  string `json:"title"`
	Query  string `json:"query"`
	Delete bool   `json:"delete"`
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Header() http.Header {
	return lrw.ResponseWriter.Header()
}

func (lrw *loggingResponseWriter) Write(d []byte) (int, error) {
	return lrw.ResponseWriter.Write(d)
}

func (lrw *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := lrw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacking not supported")
	}
	return hj.Hijack()
}

var ws = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type webContext struct {
	Request   *http.Request
	Response  http.ResponseWriter
	Config    *config.Config
	nonce     string
	csrf      string
	UserID    uint
	Username  string
	IsAdmin   bool
	userRules *config.Rules
}

func (c *webContext) effectiveRules() *config.Rules {
	if c.Config.App.UserHandling && c.userRules != nil {
		return c.userRules
	}
	return c.Config.Rules
}

func init() {
	gob.Register(uint(0))
	sub, err := iofs.Sub(static.FS, "app")
	if err != nil {
		panic(err)
	}
	staticTextFiles = make(map[string][]byte)
	appSubFS = sub
	staticFileServer = http.StripPrefix("/static/", http.FileServerFS(appSubFS))
}

func parseStaticFiles(baseDir string) error {
	files, err := static.FS.ReadDir("app")
	if err != nil {
		return err
	}
	return recParseStaticFiles(files, "app", baseDir)
	//cspHashes := make([]string, 0, len(staticTextFiles))
	//for n, c := range staticTextFiles {
	//	if strings.HasSuffix(n, ".js") {
	//		h := sha256.New()
	//		h.Write(c)
	//		s := h.Sum(nil)
	//		cspHashes = append(cspHashes, fmt.Sprintf("'sha256-%s'", base64.StdEncoding.EncodeToString(s)))
	//	}
	//}
	//cspValues = fmt.Sprintf("script-src 'strict-dynamic' %s", strings.Join(cspHashes, " "))
}

func recParseStaticFiles(entries []iofs.DirEntry, dir, baseDir string) error {
	for _, e := range entries {
		if e.IsDir() {
			subDir := path.Join(dir, e.Name())
			sd, err := static.FS.ReadDir(subDir)
			if err != nil {
				return err
			}
			if err := recParseStaticFiles(sd, subDir, baseDir); err != nil {
				return err
			}
			continue
		}
		fn := e.Name()
		if strings.HasSuffix(fn, ".html") || strings.HasSuffix(fn, ".js") || strings.HasSuffix(fn, ".css") {
			p := path.Join(dir, fn)
			c, err := static.FS.ReadFile(p)
			if err != nil {
				return err
			}
			k := strings.TrimPrefix(p, "app/")
			staticTextFiles[k] = bytes.ReplaceAll(c, []byte("/magic-string-that-we-replace-runtime-in-the-app"), []byte(baseDir))
		}
	}
	return nil
}

func Listen(cfg *config.Config) {
	sessionStore = sessions.NewCookieStore(cfg.SecretKey()[:32])
	sessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365,
		HttpOnly: true,
	}

	// This is an ugly hack required to set the base path dynamically in svelte files.
	// Svelte only supports build time specification of the base path and it accepts
	// only absolute paths: https://github.com/sveltejs/kit/issues/9569#issuecomment-3202269382
	//
	// Related issues for more details:
	//  - https://codeberg.org/asciimoo/hister/issues/7
	//  - https://github.com/asciimoo/hister/issues/147
	if err := parseStaticFiles(cfg.BasePathPrefix()); err != nil {
		panic(err)
	}

	handler := registerEndpoints(cfg)
	handler = withLogging(handler)

	log.Info().Str("Address", cfg.Server.Address).Str("Version", Version).Str("URL", cfg.BaseURL("/")).Msg("Starting webserver")
	err := http.ListenAndServe(cfg.Server.Address, handler)
	if err != nil {
		log.Error().Err(err).Msg("Webserver failed to listen on " + cfg.Server.Address)
	}
}

func registerEndpoints(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()
	tokenAuth := cfg.App.AccessToken != ""
	userHandling := cfg.App.UserHandling

	for _, e := range Endpoints {
		h := e.Handler
		if e.CSRFRequired {
			h = withCSRF(h)
		}
		if tokenAuth && !userHandling && !e.NoAuth {
			h = withTokenAuth(h)
		} else if userHandling && !e.NoAuth {
			if e.AdminOnly {
				h = withAdminAuth(h)
			} else {
				h = withUserAuth(h)
			}
		}
		mux.HandleFunc(e.Pattern(), createHandler(cfg, h))
	}
	// SPA catch-all: serve index.html for any path not matched above
	mux.HandleFunc("GET /static/", createHandler(cfg, serveStatic))
	mux.HandleFunc("GET /favicon.ico", createHandler(cfg, serveFavicon))
	mux.HandleFunc("GET /opensearch.xml", createHandler(cfg, serveOpensearch))
	mux.HandleFunc("/", createHandler(cfg, serveSPA))
	// If base_url contains a non-root path prefix (e.g. https://x.com/subfolder),
	// accept requests both with and without that prefix.
	basePrefix := cfg.BasePathPrefix()
	if basePrefix != "" {
		return withOptionalBasePathPrefix(basePrefix, mux)
	}
	return mux
}

func withOptionalBasePathPrefix(prefix string, next http.Handler) http.Handler {
	prefix = strings.TrimSuffix(prefix, "/")
	if prefix == "" || prefix == "/" {
		return next
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p != prefix && !strings.HasPrefix(p, prefix+"/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = strings.TrimPrefix(p, prefix)
		if r2.URL.Path == "" {
			r2.URL.Path = "/"
		}
		r2.RequestURI = r2.URL.RequestURI()
		next.ServeHTTP(w, r2)
	})
}

func createHandler(cfg *config.Config, h func(*webContext)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c := &webContext{
			Request:  r,
			Response: w,
			Config:   cfg,
			nonce:    rand.Text(),
		}
		if cfg.App.UserHandling {
			populateUserContext(c)
		}
		h(c)
	}
}

func withTokenAuth(handler endpointHandler) endpointHandler {
	return func(c *webContext) {
		session, err := sessionStore.Get(c.Request, storeName)
		if err != nil && session == nil {
			serve403(c)
			return
		}
		if t, ok := session.Values["access_token"].(string); ok && t == c.Config.App.AccessToken {
			handler(c)
			return
		}
		tok := c.Request.Header.Get("X-Access-Token")
		if tok == "" {
			if auth := c.Request.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				tok = strings.TrimPrefix(auth, "Bearer ")
			}
		}
		if tok != c.Config.App.AccessToken {
			serve403(c)
			return
		}
		session.Values["access_token"] = c.Config.App.AccessToken
		err = session.Save(c.Request, c.Response)
		if err != nil {
			serve500(c)
			return
		}
		handler(c)
	}
}

func populateUserContext(c *webContext) {
	session, err := sessionStore.Get(c.Request, storeName)
	if err != nil && session == nil {
		return
	}
	if uid, ok := session.Values["user_id"].(uint); ok && uid > 0 {
		c.UserID = uid
	}
	if name, ok := session.Values["username"].(string); ok {
		c.Username = name
	}
	if c.UserID == 0 {
		tok := c.Request.Header.Get("X-Access-Token")
		if tok == "" {
			if auth := c.Request.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				tok = strings.TrimPrefix(auth, "Bearer ")
			}
		}
		if tok != "" {
			if u, err := model.GetUserByToken(tok); err == nil {
				c.UserID = u.ID
				c.Username = u.Username
				c.IsAdmin = u.IsAdmin
				if rules, err := u.ParseRules(); err == nil {
					c.userRules = rules
				}
			}
		}
		return
	}
	if u, err := model.GetUserByID(c.UserID); err == nil {
		c.IsAdmin = u.IsAdmin
		if rules, err := u.ParseRules(); err == nil {
			c.userRules = rules
		}
	}
}

func withUserAuth(handler endpointHandler) endpointHandler {
	return func(c *webContext) {
		if c.UserID == 0 {
			serve403(c)
			return
		}
		handler(c)
	}
}

func withAdminAuth(handler endpointHandler) endpointHandler {
	return func(c *webContext) {
		if c.UserID == 0 {
			serve403(c)
			return
		}
		if !c.IsAdmin {
			log.Warn().Msg("Admin permission required")
			serve403(c)
			return
		}
		handler(c)
	}
}

func withCSRF(handler endpointHandler) endpointHandler {
	return func(c *webContext) {
		// Allow requests coming from the command line
		if c.Request.Header.Get("Origin") == "hister://" {
			handler(c)
			return
		}
		// Allow requests coming from the same site
		if c.Request.Header.Get("Sec-Fetch-Site") == "same-origin" {
			handler(c)
			return
		}
		// Allow add, config requests from the addons
		for _, p := range []string{"/add", "/api/add", "/api/add_pdf", "/api/config", "/api/rules", "/api/delete", "/api/label", "/api/versions"} {
			if c.Request.URL.Path != p {
				continue
			}
			if strings.HasPrefix(c.Request.Header.Get("Origin"), "moz-extension://") {
				handler(c)
				return
			}
			if c.Request.Header.Get("Origin") == "chrome-extension://cciilamhchpmbdnniabclekddabkifhb" {
				handler(c)
				return
			}
		}

		session, err := sessionStore.Get(c.Request, storeName)
		if err != nil && session == nil {
			http.Error(c.Response, err.Error(), http.StatusInternalServerError)
			return
		}
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")
		safeRequest := c.Config.IsSameHost(origin) || origin == "same-origin"
		if method != http.MethodGet && method != http.MethodHead && !safeRequest {
			sToken, ok := session.Values[tokName].(string)
			if !ok {
				http.Error(c.Response, errCSRFMismatch.Error(), http.StatusInternalServerError)
				return
			}
			token := c.Request.PostFormValue(tokName)
			if token == "" {
				token = c.Request.Header.Get("X-CSRF-Token")
			}
			if token != sToken {
				http.Error(c.Response, errCSRFMismatch.Error(), http.StatusInternalServerError)
				return
			}
		}
		tok := rand.Text()
		session.Values[tokName] = tok
		err = session.Save(c.Request, c.Response)
		if err != nil {
			http.Error(c.Response, err.Error(), http.StatusInternalServerError)
			return
		}
		c.csrf = tok
		c.Response.Header().Add("X-CSRF-Token", tok)
		handler(c)
	}
}

func withLogging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{w, http.StatusOK}
		h.ServeHTTP(lrw, r)
		log.Info().Str("Method", r.Method).Int("Status", lrw.statusCode).Dur("LoadTimeMS", time.Since(start)).Str("URL", r.RequestURI).Msg("WEB")
	})
}

func serveIndex(c *webContext) {
	content, ok := staticTextFiles["index.html"]
	if !ok {
		serve500(c)
		return
	}
	c.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Response.Header().Set("Content-Security-Policy", fmt.Sprintf("script-src 'strict-dynamic' 'nonce-%s'", c.nonce))
	if _, err := c.Response.Write(bytes.ReplaceAll(content, []byte("<script>"), fmt.Appendf(nil, `<script nonce="%s">`, c.nonce))); err != nil {
		log.Warn().Err(err).Msg("failed to write index response")
	}
}

// serveSPA serves the SPA index.html for any route not matching a static file.
func serveSPA(c *webContext) {
	path := strings.TrimPrefix(c.Request.URL.Path, "/")
	if path == "index.html" {
		serveIndex(c)
		return
	}
	if content, ok := staticTextFiles[path]; ok {
		ext := filepath.Ext(path)
		if mimeType := mime.TypeByExtension(ext); mimeType != "" {
			c.Response.Header().Set("Content-Type", mimeType)
		} else {
			// Default to application/octet-stream if we can't detect the type
			c.Response.Header().Set("Content-Type", "application/octet-stream")
		}
		c.Response.WriteHeader(http.StatusOK)
		if _, err := c.Response.Write(content); err != nil {
			log.Warn().Err(err).Msg("failed to write static text response")
		}
		return
	}
	// If the exact file exists in the embedded app FS, serve it directly
	if _, err := iofs.Stat(appSubFS, path); err == nil {
		// Read the file and serve it with proper MIME type
		content, err := iofs.ReadFile(appSubFS, path)
		if err != nil {
			serve500(c)
			return
		}
		// Detect and set proper MIME type
		ext := filepath.Ext(path)
		if mimeType := mime.TypeByExtension(ext); mimeType != "" {
			c.Response.Header().Set("Content-Type", mimeType)
		} else {
			// Default to application/octet-stream if we can't detect the type
			c.Response.Header().Set("Content-Type", "application/octet-stream")
		}
		c.Response.WriteHeader(http.StatusOK)
		if _, err := c.Response.Write(content); err != nil {
			log.Warn().Err(err).Msg("failed to write static file response")
		}
		return
	}

	// redirect to configured search engine if the query starts or ends with "!!"
	q := c.Request.URL.Query().Get("q")
	if strings.HasPrefix(q, "!!") || strings.HasSuffix(q, "!!") {
		if strings.HasPrefix(q, "!!") {
			q = q[2:]
		} else if strings.HasSuffix(q, "!!") {
			q = q[:len(q)-2]
		}
		c.Redirect(strings.Replace(c.Config.App.SearchURL, "{query}", strings.TrimSpace(q), 1))
		return
	}

	// redirect to configured search engine if query string exists but we have no matching results
	if q != "" && c.Config.App.RedirectOnNoResults {
		res, err := indexer.Search(c.Config, &indexer.Query{
			Text:   c.effectiveRules().ResolveAliases(q),
			UserID: c.UserID,
		})
		if err != nil {
			res = &indexer.Results{}
		}
		hr, err := model.GetURLsByQuery(c.UserID, q)
		if err == nil && len(hr) > 0 {
			res.History = hr
		}
		if err != nil {
			serve500(c)
			return
		}
		if len(res.Documents) == 0 && len(hr) == 0 {
			c.Redirect(strings.Replace(c.Config.App.SearchURL, "{query}", q, 1))
			return
		}
	}
	// Otherwise serve index.html for client-side routing
	serveIndex(c)
}

func serveLogin(c *webContext) {
	if c.Config.Server.OAuthOnly {
		http.Error(c.Response, "password login disabled, use OAuth", http.StatusForbidden)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		serve500(c)
		return
	}
	user, err := model.AuthenticateUser(req.Username, req.Password)
	if err != nil {
		http.Error(c.Response, "invalid credentials", http.StatusUnauthorized)
		return
	}
	session, err := sessionStore.Get(c.Request, storeName)
	if err != nil && session == nil {
		serve500(c)
		return
	}
	session.Values["user_id"] = user.ID
	session.Values["username"] = user.Username
	if err := session.Save(c.Request, c.Response); err != nil {
		serve500(c)
		return
	}
	c.JSON(map[string]string{"username": user.Username})
}

func serveLogout(c *webContext) {
	session, err := sessionStore.Get(c.Request, storeName)
	if err != nil && session == nil {
		serve500(c)
		return
	}
	delete(session.Values, "user_id")
	delete(session.Values, "username")
	if err := session.Save(c.Request, c.Response); err != nil {
		serve500(c)
		return
	}
	serve200(c)
}

func serveProfile(c *webContext) {
	if c.Config.App.UserHandling {
		resp := map[string]any{
			"user_id":  c.UserID,
			"username": c.Username,
			"is_admin": c.IsAdmin,
		}
		if c.IsAdmin {
			resp["version"] = Version
		}
		c.JSON(resp)
		return
	}
	serve200(c)
}

func serveGenerateToken(c *webContext) {
	token, err := model.RegenerateToken(c.UserID)
	if err != nil {
		serve500(c)
		return
	}
	c.JSON(map[string]string{"token": token})
}

// serveConfig returns app configuration as JSON and refreshes CSRF token.
func serveConfig(c *webContext) {
	type configResponse struct {
		BaseURL             string            `json:"baseUrl"`
		BasePath            string            `json:"basePath"`
		WsURL               string            `json:"wsUrl"`
		SearchURL           string            `json:"searchUrl"`
		OpenResultsOnNewTab bool              `json:"openResultsOnNewTab"`
		Hotkeys             map[string]string `json:"hotkeys"`
		AuthMode            string            `json:"authMode"`
		Authenticated       bool              `json:"authenticated"`
		Username            string            `json:"username,omitempty"`
		UserID              uint              `json:"userId,omitempty"`
		SemanticEnabled     bool              `json:"semanticEnabled"`
		SemanticWeight      float64           `json:"semanticWeight,omitempty"`
		SimilarityThreshold float64           `json:"similarityThreshold,omitempty"`
		OAuthProviders      []string          `json:"oauthProviders,omitempty"`
		OAuthOnly           bool              `json:"oauthOnly,omitempty"`
		DisablePreviews     bool              `json:"disablePreviews,omitempty"`
	}
	authMode := "none"
	authenticated := true
	if c.Config.App.UserHandling {
		authMode = "user"
		authenticated = c.UserID > 0
	} else if c.Config.App.AccessToken != "" {
		authMode = "token"
		// Check whether this request carries a valid token via session or header.
		if session, err := sessionStore.Get(c.Request, storeName); err == nil && session != nil {
			if t, ok := session.Values["access_token"].(string); ok && t == c.Config.App.AccessToken {
				authenticated = true
			} else if c.Request.Header.Get("X-Access-Token") == c.Config.App.AccessToken {
				authenticated = true
			} else {
				authenticated = false
			}
		} else {
			authenticated = c.Request.Header.Get("X-Access-Token") == c.Config.App.AccessToken
		}
	}
	hotkeys := c.Config.Hotkeys.Web
	if hotkeys == nil {
		hotkeys = make(map[string]string)
	}
	oauthProviders := make([]string, 0, len(c.Config.Server.OAuth))
	for name := range c.Config.Server.OAuth {
		oauthProviders = append(oauthProviders, name)
	}
	c.JSON(configResponse{
		BaseURL:             c.Config.BaseURL(""),
		BasePath:            c.Config.BasePathPrefix(),
		WsURL:               c.Config.WebSocketURL(),
		SearchURL:           c.Config.App.SearchURL,
		OpenResultsOnNewTab: c.Config.App.OpenResultsOnNewTab,
		Hotkeys:             hotkeys,
		AuthMode:            authMode,
		Authenticated:       authenticated,
		Username:            c.Username,
		UserID:              c.UserID,
		SemanticEnabled:     indexer.SemanticSearchEnabled(),
		SemanticWeight:      c.Config.SemanticSearch.SemanticWeight,
		SimilarityThreshold: c.Config.SemanticSearch.SimilarityThreshold,
		OAuthProviders:      oauthProviders,
		OAuthOnly:           c.Config.Server.OAuthOnly,
		DisablePreviews:     c.Config.App.DisablePreviews,
	})
}

// parseSearchQueryParams parses URL query parameters into an indexer.Query.
func parseSearchQueryParams(r *http.Request) (*indexer.Query, error) {
	urlParams := r.URL.Query()
	query := &indexer.Query{}
	if rawQuery := urlParams.Get("query"); rawQuery != "" {
		if err := json.Unmarshal([]byte(rawQuery), query); err != nil {
			return nil, err
		}
	}
	if q := urlParams.Get("q"); q != "" {
		query.Text = q
	}
	for param, field := range map[string]*int64{"date_from": &query.DateFrom, "date_to": &query.DateTo} {
		if v := urlParams.Get(param); v != "" {
			if t, err := time.Parse("2006-01-02", v); err == nil {
				ts := t.Unix()
				if param == "date_to" {
					// Include the entire end date by advancing to end of day (23:59:59)
					ts += 24*60*60 - 1
				}
				*field = ts
			}
		}
	}
	if urlParams.Get("include_html") == "1" {
		query.IncludeHTML = true
	}
	if pk := urlParams.Get("page_key"); pk != "" {
		query.PageKey = pk
	}
	if s := urlParams.Get("sort"); s != "" {
		query.Sort = s
	}
	if v := urlParams.Get("semantic"); v != "" {
		query.SemanticEnabled = v == "1" || v == "true"
	}
	if v := urlParams.Get("semantic_threshold"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			query.SemanticThreshold = f
		}
	}
	return query, nil
}

// serveSearchHTTP executes a single search and writes a JSON response.
func serveSearchHTTP(c *webContext, query *indexer.Query) {
	r, err := doSearch(query, c.Config, c.effectiveRules(), c.UserID)
	if err != nil {
		log.Error().Err(err).Msg("search error")
		serve500(c)
		return
	}
	jr, err := json.Marshal(r)
	if err != nil {
		serve500(c)
		return
	}
	c.Response.Header().Add("Content-Type", "application/json")
	if _, err := c.Response.Write(jr); err != nil {
		log.Warn().Err(err).Msg("failed to write search response")
	}
}

// serveSearchWebSocket upgrades the connection and serves searches over WebSocket.
func serveSearchWebSocket(c *webContext) {
	conn, err := ws.Upgrade(c.Response, c.Request, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to upgrade websocket request")
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close websocket connection")
		}
	}()
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Msg("failed to read websocket message")
			}
			break
		}
		var query *indexer.Query
		if err = json.Unmarshal(msg, &query); err != nil {
			log.Error().Err(err).Msg("failed to parse query")
			continue
		}
		// Semantic search is only available when the server has it enabled;
		// otherwise honour the client's per-request flag.
		if !c.Config.SemanticSearch.Enable {
			query.SemanticEnabled = false
		}
		res, err := doSearch(query, c.Config, c.effectiveRules(), c.UserID)
		if err != nil {
			log.Error().Err(err).Msg("search error")
			continue
		}
		jr, err := json.Marshal(res)
		if err != nil {
			log.Error().Err(err).Msg("failed to marshal indexer results")
			continue
		}
		if err := conn.WriteMessage(websocket.TextMessage, jr); err != nil {
			log.Error().Err(err).Msg("failed to write websocket message")
			break
		}
	}
}

func serveSearch(c *webContext) {
	origin := c.Request.Header.Get("Origin")
	// `?format=json` (or `Accept: application/json`) opts the caller into a
	// pure JSON response and skips the same-host Origin check, so CLI tools
	// and ad-hoc HTTP clients can hit the search endpoint without spoofing
	// a hister:// Origin header.
	jsonFormat := c.Request.URL.Query().Get("format") == "json" ||
		c.Request.Header.Get("Accept") == "application/json"
	if !jsonFormat && !c.Config.IsSameHost(origin) {
		serve500(c)
		log.Info().Str("Origin", origin).Msg("Invalid origin")
		return
	}
	query, err := parseSearchQueryParams(c.Request)
	if err != nil {
		c.Response.WriteHeader(http.StatusBadRequest)
		return
	}
	if jsonFormat && query.Text == "" {
		c.Response.WriteHeader(http.StatusBadRequest)
		_, _ = c.Response.Write([]byte(`{"error":"text query required for format=json"}`))
		return
	}
	if query.Text != "" {
		serveSearchHTTP(c, query)
		return
	}
	serveSearchWebSocket(c)
}

func doSearch(query *indexer.Query, cfg *config.Config, rules *config.Rules, userID uint) (*indexer.Results, error) {
	start := time.Now()
	oq := query.Text
	query.Text = rules.ResolveAliases(query.Text)
	query.UserID = userID
	if rules != nil && rules.Priority != nil {
		query.PriorityPatterns = rules.Priority.ReStrs
	}
	res, err := indexer.Search(cfg, query)
	if err != nil {
		log.Error().Err(err).Msg("failed to get indexer results")
	}
	if res == nil {
		res = &indexer.Results{}
	}
	hr, err := model.GetURLsByQuery(userID, oq)
	if err == nil && len(hr) > 0 {
		res.History = hr
		priorityByURL := make(map[string]*model.URLCount, len(hr))
		for _, h := range hr {
			priorityByURL[h.URL] = h
		}
		filtered := res.Documents[:0]
		for _, d := range res.Documents {
			if h, ok := priorityByURL[d.URL]; ok {
				if h.Text == "" {
					h.Text = d.Text
				}
				continue
			}
			filtered = append(filtered, d)
		}
		res.Documents = filtered
	}
	if oq != "" {
		res.QuerySuggestion = model.GetQuerySuggestion(userID, oq)
	}
	duration := float32(time.Since(start).Milliseconds()) / 1000.
	res.SearchDuration = fmt.Sprintf("%.3f seconds", duration)
	return res, nil
}

// computeDocumentDiff returns a diff-match-patch patch string representing the
// changes between old and new. It diffs HTML when both documents have HTML,
// otherwise it diffs the plain text fields. Returns an empty string when the
// content is identical.
// computeDocumentDiff returns unified diff-match-patch patch strings for the
// HTML and Text fields independently. Either return value may be empty when
// the corresponding content is absent or identical between the two versions.
// applyPatchReverse reconstructs an older document by inverting the stored
// patch and applying the result to the current content. The diff-match-patch
// Patch struct has an unexported diffs field, so we work at the text level:
// swap the position fields in each @@ header and swap every leading '-'/'+'
// marker on diff lines. PatchApply uses fuzzy matching so position drift
// across multiple sequential applications is handled automatically.
// On any error it returns the content unchanged.
func applyPatchReverse(patchText, content string) string {
	if patchText == "" {
		return content
	}
	lines := strings.Split(patchText, "\n")
	reversed := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "@@ "):
			// "@@ -a,b +c,d @@" → "@@ -c,d +a,b @@"
			parts := strings.SplitN(line, " ", 4)
			if len(parts) == 4 && strings.HasPrefix(parts[1], "-") && strings.HasPrefix(parts[2], "+") {
				reversed = append(reversed, "@@ -"+parts[2][1:]+" +"+parts[1][1:]+" @@")
			} else {
				reversed = append(reversed, line)
			}
		case strings.HasPrefix(line, "+"):
			reversed = append(reversed, "-"+line[1:])
		case strings.HasPrefix(line, "-"):
			reversed = append(reversed, "+"+line[1:])
		default:
			reversed = append(reversed, line)
		}
	}
	dmp := diffmatchpatch.New()
	patches, err := dmp.PatchFromText(strings.Join(reversed, "\n"))
	if err != nil {
		return content
	}
	result, _ := dmp.PatchApply(patches, content)
	return result
}

func computeDocumentDiff(old, new *document.Document) (htmlDiff, textDiff string) {
	dmp := diffmatchpatch.New()
	makePatch := func(oldContent, newContent string) string {
		if oldContent == newContent {
			return ""
		}
		diffs := dmp.DiffMain(oldContent, newContent, true)
		diffs = dmp.DiffCleanupSemantic(diffs)
		return dmp.PatchToText(dmp.PatchMake(oldContent, diffs))
	}
	htmlDiff = makePatch(old.HTML, new.HTML)
	textDiff = makePatch(old.Text, new.Text)
	return
}

// serveVersions returns all stored version diffs for a given URL and the
// authenticated user (or user 0 when user handling is disabled).
func serveVersions(c *webContext) {
	u := c.Request.URL.Query().Get("url")
	if u == "" {
		http.Error(c.Response, "url parameter is required", http.StatusBadRequest)
		return
	}
	versions, err := model.GetDocumentVersions(u, c.UserID)
	if err != nil {
		log.Error().Err(err).Str("url", u).Msg("failed to get document versions")
		serve500(c)
		return
	}
	c.JSON(versions)
}

func serveAdd(c *webContext) {
	m := c.Request.Method
	if m == http.MethodGet {
		serve200(c)
		return
	}
	if m != http.MethodPost {
		serve500(c)
		return
	}
	d := &document.Document{}
	jsonData := false
	if strings.Contains(c.Request.Header.Get("Content-Type"), "json") {
		jsonData = true
		err := json.NewDecoder(c.Request.Body).Decode(d)
		if err != nil {
			serve500(c)
			return
		}
	} else {
		err := c.Request.ParseForm()
		if err != nil {
			serve500(c)
			return
		}
		f := c.Request.PostForm
		d.URL = f.Get("url")
		d.Title = f.Get("title")
		d.Text = f.Get("text")
	}
	if !c.effectiveRules().IsSkip(d.URL) && !c.Config.IsSameHost(d.URL) {
		d.UserID = c.UserID
		if c.Config.App.UserHandling && c.IsAdmin {
			if h := c.Request.Header.Get("X-Hister-Target-User-ID"); h != "" {
				if uid, err := strconv.ParseUint(h, 10, 64); err == nil {
					d.UserID = uint(uid)
				}
			}
		}
		rules := c.effectiveRules()
		var existingDoc *document.Document
		if rules.IsVersioning(d.URL) {
			existingDoc = indexer.GetByURLAndUser(d.URL, d.UserID)
		}
		err := indexer.Add(d)
		if err != nil {
			if errors.Is(err, document.ErrSensitiveContent) {
				log.Warn().Str("URL", d.URL).Msg("rejected document: sensitive content")
				http.Error(c.Response, document.ErrSensitiveContent.Error(), http.StatusUnprocessableEntity)
				return
			}
			log.Error().Err(err).Str("URL", d.URL).Msg("failed to create index")
			serve500(c)
			return
		}
		if existingDoc != nil {
			htmlDiff, textDiff := computeDocumentDiff(existingDoc, d)
			if htmlDiff != "" || textDiff != "" {
				if err := model.SaveDocumentVersion(d.URL, d.UserID, htmlDiff, textDiff); err != nil {
					log.Warn().Err(err).Str("url", d.URL).Msg("failed to save document version")
				}
			}
		}
		c.Response.WriteHeader(http.StatusCreated)
	} else {
		log.Debug().Str("url", d.URL).Msg("skip indexing")
		c.Response.WriteHeader(http.StatusNotAcceptable)
	}
	if jsonData {
		return
	}
	serve200(c)
}

func serveAddPDF(c *webContext) {
	if c.Request.Method != http.MethodPost {
		serve500(c)
		return
	}

	var req struct {
		Document *document.Document `json:"document"`
		PDF      string             `json:"pdf"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		http.Error(c.Response, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Document == nil {
		http.Error(c.Response, "missing document field", http.StatusBadRequest)
		return
	}
	if req.PDF == "" {
		http.Error(c.Response, "missing pdf field", http.StatusBadRequest)
		return
	}
	pdfData, err := base64.StdEncoding.DecodeString(req.PDF)
	if err != nil {
		http.Error(c.Response, "pdf must be base64-encoded: "+err.Error(), http.StatusBadRequest)
		return
	}

	d := req.Document
	if c.effectiveRules().IsSkip(d.URL) || c.Config.IsSameHost(d.URL) {
		log.Debug().Str("url", d.URL).Msg("skip indexing pdf")
		c.Response.WriteHeader(http.StatusNotAcceptable)
		return
	}

	d.UserID = c.UserID
	if c.Config.App.UserHandling && c.IsAdmin {
		if h := c.Request.Header.Get("X-Hister-Target-User-ID"); h != "" {
			if uid, err := strconv.ParseUint(h, 10, 64); err == nil {
				d.UserID = uint(uid)
			}
		}
	}

	if err := indexer.AddPDF(d, pdfData); err != nil {
		if errors.Is(err, document.ErrSensitiveContent) {
			log.Warn().Str("URL", d.URL).Msg("rejected pdf document: sensitive content")
			http.Error(c.Response, document.ErrSensitiveContent.Error(), http.StatusUnprocessableEntity)
			return
		}
		log.Error().Err(err).Str("URL", d.URL).Msg("failed to index pdf")
		serve500(c)
		return
	}

	log.Debug().Str("URL", d.URL).Msg("pdf added to index")
	c.Response.WriteHeader(http.StatusCreated)
}

func serveUpdateLabel(c *webContext) {
	var req struct {
		URL   string `json:"url"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		http.Error(c.Response, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(c.Response, "missing url", http.StatusBadRequest)
		return
	}
	doc := indexer.GetByURLAndUser(req.URL, c.UserID)
	if doc == nil {
		http.Error(c.Response, "document not found", http.StatusNotFound)
		return
	}
	doc.Label = req.Label
	if err := indexer.Save(doc); err != nil {
		log.Error().Err(err).Str("url", req.URL).Msg("failed to save label")
		serve500(c)
		return
	}
	c.JSON(map[string]any{"ok": true})
}

func serveHistory(c *webContext) {
	rssFormat := c.Request.URL.Query().Get("format") == "rss"
	if c.Request.URL.Query().Get("opened") == "true" {
		var lastID uint
		if v := c.Request.URL.Query().Get("last_id"); v != "" {
			if parsed, err := strconv.ParseUint(v, 10, 64); err == nil {
				lastID = uint(parsed)
			}
		}
		items, err := model.GetLatestHistoryItems(c.UserID, 100, lastID)
		if err != nil {
			serve500(c)
			return
		}
		type openedItem struct {
			ID    uint   `json:"id"`
			URL   string `json:"url"`
			Title string `json:"title"`
			Query string `json:"query"`
			Added int64  `json:"added"`
		}
		type openedResponse struct {
			Documents []*openedItem `json:"documents"`
			LastID    uint          `json:"last_id"`
		}
		docs := make([]*openedItem, 0, len(items))
		for _, item := range items {
			docs = append(docs, &openedItem{
				ID:    item.ID,
				URL:   item.URL,
				Title: item.Title,
				Query: item.Query,
				Added: item.UpdatedAt.Unix(),
			})
		}
		var nextLastID uint
		if len(docs) > 0 {
			nextLastID = docs[len(docs)-1].ID
		}
		if rssFormat {
			rssItems := make([]rssItem, 0, len(docs))
			for _, d := range docs {
				title := d.Title
				if title == "" {
					title = d.URL
				}
				rssItems = append(rssItems, rssItem{
					Title:   title,
					Link:    d.URL,
					GUID:    rssGUID{Value: d.URL, IsPermaLink: "true"},
					PubDate: time.Unix(d.Added, 0).UTC().Format(time.RFC1123Z),
				})
			}
			serveRSS(c, "Hister - visited pages", c.Config.BaseURL("/"), rssItems)
			return
		}
		c.JSON(&openedResponse{Documents: docs, LastID: nextLastID})
		return
	}
	ds := indexer.GetLatestDocuments(100, c.Request.URL.Query().Get("last"), c.UserID)
	if rssFormat {
		var rssItems []rssItem
		if ds != nil {
			rssItems = make([]rssItem, 0, len(ds.Documents))
			for _, d := range ds.Documents {
				title := d.Title
				if title == "" {
					title = d.URL
				}
				rssItems = append(rssItems, rssItem{
					Title:   title,
					Link:    d.URL,
					GUID:    rssGUID{Value: d.URL, IsPermaLink: "true"},
					PubDate: time.Unix(d.Added, 0).UTC().Format(time.RFC1123Z),
				})
			}
		}
		serveRSS(c, "Hister - indexed pages", c.Config.BaseURL("/"), rssItems)
		return
	}
	c.JSON(ds)
}

type rssGUID struct {
	Value       string `xml:",chardata"`
	IsPermaLink string `xml:"isPermaLink,attr,omitempty"`
}

type rssItem struct {
	Title   string  `xml:"title"`
	Link    string  `xml:"link"`
	GUID    rssGUID `xml:"guid"`
	PubDate string  `xml:"pubDate,omitempty"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

func serveRSS(c *webContext, title, link string, items []rssItem) {
	feed := rssFeed{
		Version: "2.0",
		Channel: rssChannel{
			Title:       title,
			Link:        link,
			Description: title,
			Items:       items,
		},
	}
	c.Response.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	if _, err := c.Response.Write([]byte(xml.Header)); err != nil {
		log.Warn().Err(err).Msg("failed to write RSS header")
		return
	}
	if err := xml.NewEncoder(c.Response).Encode(feed); err != nil {
		log.Warn().Err(err).Msg("failed to encode RSS feed")
	}
}

func serveSaveHistory(c *webContext) {
	h := &historyItem{}
	err := json.NewDecoder(c.Request.Body).Decode(h)
	if err != nil {
		serve500(c)
		return
	}
	if h.Delete {
		if err := model.DeleteHistoryItem(c.UserID, h.Query, h.URL); err != nil {
			serve500(c)
		}
		return
	}
	err = model.UpdateHistory(c.UserID, strings.TrimSpace(h.Query), strings.TrimSpace(h.URL), strings.TrimSpace(h.Title))
	if err != nil {
		log.Error().Err(err).Msg("failed to update history")
		serve500(c)
		return
	}
}

// validatePatterns checks that each string in patterns is a valid Go regexp.
// Returns an error naming the first invalid pattern.
func validatePatterns(patterns []string) error {
	for _, p := range patterns {
		if _, err := regexp.Compile(p); err != nil {
			return fmt.Errorf("invalid pattern %q: %w", p, err)
		}
	}
	return nil
}

func serveRules(c *webContext) {
	m := c.Request.Method
	rules := c.effectiveRules()
	if m == http.MethodGet {
		type rulesResponse struct {
			Skip       []string          `json:"skip"`
			Priority   []string          `json:"priority"`
			Versioning []string          `json:"versioning"`
			Aliases    map[string]string `json:"aliases"`
		}
		skip := rules.Skip.ReStrs
		if skip == nil {
			skip = []string{}
		}
		priority := rules.Priority.ReStrs
		if priority == nil {
			priority = []string{}
		}
		versioning := rules.Versioning.ReStrs
		if versioning == nil {
			versioning = []string{}
		}
		aliases := map[string]string(rules.Aliases)
		if aliases == nil {
			aliases = make(map[string]string)
		}
		c.JSON(rulesResponse{Skip: skip, Priority: priority, Versioning: versioning, Aliases: aliases})
		return
	}
	if m != http.MethodPost {
		serve500(c)
		return
	}
	err := c.Request.ParseForm()
	if err != nil {
		serve500(c)
		return
	}
	f := c.Request.PostForm
	skipPatterns := uniqueStrings(strings.Fields(f.Get("skip")))
	priorityPatterns := uniqueStrings(strings.Fields(f.Get("priority")))
	versioningPatterns := uniqueStrings(strings.Fields(f.Get("versioning")))
	for label, patterns := range map[string][]string{
		"skip":       skipPatterns,
		"priority":   priorityPatterns,
		"versioning": versioningPatterns,
	} {
		if err := validatePatterns(patterns); err != nil {
			http.Error(c.Response, fmt.Sprintf("%s: %s", label, err.Error()), http.StatusBadRequest)
			return
		}
	}
	rules.Skip.ReStrs = skipPatterns
	rules.Priority.ReStrs = priorityPatterns
	if rules.Versioning == nil {
		rules.Versioning = &config.Rule{ReStrs: make([]string, 0)}
	}
	rules.Versioning.ReStrs = versioningPatterns
	if err := rules.Compile(); err != nil {
		log.Error().Err(err).Msg("failed to compile rules")
		serve500(c)
		return
	}
	if c.Config.App.UserHandling {
		if err := model.SaveUserRules(c.UserID, rules); err != nil {
			log.Error().Err(err).Msg("failed to save user rules")
			serve500(c)
			return
		}
		c.userRules = rules
	} else {
		if err := c.Config.SaveRules(); err != nil {
			log.Error().Err(err).Msg("failed to save rules")
			serve500(c)
			return
		}
	}
	serve200(c)
}

func serveGetFacets(c *webContext) {
	params := c.Request.URL.Query()
	q := &indexer.Query{
		Text:       params.Get("q"),
		Facets:     true,
		FacetsOnly: true,
		FacetSizes: make(map[string]int),
	}
	for param, field := range map[string]*int64{"date_from": &q.DateFrom, "date_to": &q.DateTo} {
		if v := params.Get(param); v != "" {
			if t, err := strconv.ParseInt(v, 10, 64); err == nil {
				*field = t
			}
		}
	}
	for key, vals := range params {
		if name, ok := strings.CutPrefix(key, "size_"); ok && len(vals) > 0 {
			if n, err := strconv.Atoi(vals[0]); err == nil && n > 0 {
				q.FacetSizes[name] = n
			}
		}
	}
	res, err := doSearch(q, c.Config, c.effectiveRules(), c.UserID)
	if err != nil || res.Facets == nil {
		c.JSON(map[string]any{})
		return
	}
	c.JSON(res.Facets)
}

func serveGet(c *webContext) {
	u := c.Request.URL.Query().Get("url")
	doc := indexer.GetByURLAndUser(u, c.UserID)
	if doc == nil {
		http.Error(c.Response, "document not found", http.StatusNotFound)
		return
	}
	// We skip generating the body on HEAD requests, since those only check the status.
	// Note that we want to return the same status as a GET request, so **no faillible processing**
	// is to be made inside of this block!
	if c.Request.Method != "HEAD" {
		c.JSON(doc)
	}
}

func servePreview(c *webContext) {
	u := c.Request.URL.Query().Get("url")
	extractorName := c.Request.URL.Query().Get("extractor")
	versionIDStr := c.Request.URL.Query().Get("version")
	doc := indexer.GetByURLAndUser(u, c.UserID)
	if doc == nil {
		serve500(c)
		return
	}
	// If a specific version is requested, reconstruct the older document content
	// by applying the stored diffs in reverse order.
	var viewingVersionID uint
	var viewingVersionCreatedAt time.Time
	if versionIDStr != "" {
		if versionID, err := strconv.ParseUint(versionIDStr, 10, 64); err == nil {
			if versions, err := model.GetDocumentVersionsUntil(u, c.UserID, uint(versionID)); err == nil && len(versions) > 0 {
				docCopy := *doc
				for _, v := range versions {
					docCopy.HTML = applyPatchReverse(v.HTMLDiff, docCopy.HTML)
					docCopy.Text = applyPatchReverse(v.TextDiff, docCopy.Text)
				}
				doc = &docCopy
				// versions is ordered newest first; the last element is the target version.
				target := versions[len(versions)-1]
				viewingVersionID = target.ID
				viewingVersionCreatedAt = target.CreatedAt
			}
		}
	}
	var resp types.PreviewResponse
	var err error
	if doc.HTML == "" {
		resp = types.PreviewResponse{Content: doc.Text}
	} else {
		resp, err = extractor.Preview(doc, extractorName)
		if err != nil {
			if errors.Is(err, extractor.ErrNoExtractor) {
				http.Error(c.Response, err.Error(), http.StatusBadRequest)
				return
			}
			log.Warn().Err(err).Str("url", u).Msg("failed to generate preview")
			serve500(c)
			return
		}
	}
	payload := map[string]any{
		"title":    doc.Title,
		"content":  resp.Content,
		"template": resp.Template,
		"added":    doc.Added,
	}
	if viewingVersionID > 0 {
		payload["version_id"] = viewingVersionID
		payload["version_created_at"] = viewingVersionCreatedAt
	}
	if versionCount, err := model.CountDocumentVersions(u, c.UserID); err == nil && versionCount > 0 {
		payload["version_count"] = versionCount
	}
	if meta := doc.GetPreviewMeta(); meta != nil {
		payload["meta"] = meta
	}
	c.JSON(payload)
}

func serveFile(c *webContext) {
	filePath := c.Request.URL.Query().Get("path")
	if filePath == "" {
		http.Error(c.Response, "missing path parameter", http.StatusBadRequest)
		return
	}

	// Resolve to absolute and clean the path to prevent traversal
	filePath = filepath.Clean(filePath)
	if !filepath.IsAbs(filePath) {
		http.Error(c.Response, "path must be absolute", http.StatusBadRequest)
		return
	}

	// Resolve symlinks to prevent a symlink inside a configured directory
	// from serving files outside it
	resolvedPath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		http.Error(c.Response, "file not found", http.StatusNotFound)
		return
	}

	// Verify the resolved file is within a configured directory (also resolved)
	allowed := false
	for _, dir := range c.Config.Indexer.Directories {
		expandedDir := filepath.Clean(files.ExpandHome(dir.Path))
		resolvedDir, err := filepath.EvalSymlinks(expandedDir)
		if err != nil {
			continue
		}
		if files.HasPathPrefix(resolvedPath, resolvedDir) {
			allowed = true
			break
		}
	}
	if !allowed {
		http.Error(c.Response, "file not in configured directories", http.StatusForbidden)
		return
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(c.Response, "file not found", http.StatusNotFound)
		return
	}

	ext := filepath.Ext(filePath)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "text/plain; charset=utf-8"
	}
	c.Response.Header().Set("Content-Type", mimeType)
	if _, err := c.Response.Write(content); err != nil {
		log.Warn().Err(err).Msg("failed to write file response")
	}
}

func serveAPI(c *webContext) {
	type endpointInfo struct {
		Name         string             `json:"name"`
		Path         string             `json:"path"`
		Method       string             `json:"method"`
		CSRFRequired bool               `json:"csrf_required"`
		Description  string             `json:"description"`
		Args         []*EndpointArg     `json:"args,omitempty"`
		JSONSchema   []*JSONSchemaField `json:"json_schema,omitempty"`
	}
	var result []endpointInfo
	for _, e := range Endpoints {
		result = append(result, endpointInfo{
			Name:         e.Name,
			Path:         e.Path,
			Method:       e.Method,
			CSRFRequired: e.CSRFRequired,
			Description:  e.Description,
			Args:         e.Args,
			JSONSchema:   e.JSONSchema,
		})
	}
	c.JSON(result)
}

func serveStats(c *webContext) {
	hs, _ := model.GetLatestHistoryItems(c.UserID, 5, 0)
	var docCount uint64
	if c.Config.App.UserHandling {
		docCount = indexer.DocumentCountByUser(c.UserID)
	} else {
		docCount = indexer.DocumentCount()
	}
	rules := c.effectiveRules()
	c.JSON(map[string]any{
		"doc_count":       docCount,
		"rule_count":      rules.Count(),
		"alias_count":     len(rules.Aliases),
		"recent_searches": hs,
	})
}

func serveExtractors(c *webContext) {
	u := c.Request.URL.Query().Get("url")
	if u != "" {
		doc := indexer.GetByURLAndUser(u, c.UserID)
		if doc == nil {
			serve500(c)
			return
		}
		c.JSON(extractor.ListMatching(doc))
		return
	}
	infos := extractor.List()
	if !c.Config.App.DisplayExtractorConfig {
		for i := range infos {
			infos[i].Options = nil
		}
	}
	c.JSON(infos)
}

func serveOpensearch(c *webContext) {
	baseURL := strings.TrimSuffix(c.Config.BaseURL("/"), "/")
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/">
  <ShortName>Hister</ShortName>
  <Description>Search your history with Hister</Description>
  <Url type="text/html" template="%s/?q={searchTerms}"/>
  <Url type="application/x-suggestions+json" template="%s/suggest?q={searchTerms}"/>
</OpenSearchDescription>`, baseURL, baseURL)
	c.Response.Header().Set("Content-Type", "application/xml")
	if _, err := c.Response.Write([]byte(xml)); err != nil {
		log.Warn().Err(err).Msg("failed to write opensearch response")
	}
}

const suggestLimit = 10

func serveSuggest(c *webContext) {
	// Sec-Fetch-Site is set by browsers and forbidden to JS, so a cross-site
	// fetch() can't spoof it. Browser address-bar flows either omit the header
	// (Firefox) or send "none" (Chrome); reject anything explicitly cross-site.
	switch c.Request.Header.Get("Sec-Fetch-Site") {
	case "", "none", "same-origin", "same-site":
	default:
		c.Response.WriteHeader(http.StatusForbidden)
		return
	}
	q := c.Request.URL.Query().Get("q")
	suggestions := []string{}
	if q != "" {
		res, err := indexer.Search(c.Config, &indexer.Query{
			Text:   c.effectiveRules().ResolveAliases(q),
			UserID: c.UserID,
			Limit:  suggestLimit,
		})
		if err != nil {
			log.Warn().Err(err).Msg("suggest search failed")
		}
		if res != nil {
			for _, d := range res.Documents {
				title := strings.TrimSpace(d.Title)
				if title == "" {
					title = d.URL
				}
				suggestions = append(suggestions, title)
			}
		}
	}
	jr, err := json.Marshal([]any{q, suggestions})
	if err != nil {
		log.Warn().Err(err).Msg("failed to marshal suggest response")
		return
	}
	c.Response.Header().Set("Content-Type", "application/x-suggestions+json")
	if _, err := c.Response.Write(jr); err != nil {
		log.Warn().Err(err).Msg("failed to write suggest response")
	}
}

func serveAddAlias(c *webContext) {
	err := c.Request.ParseForm()
	if err != nil {
		serve500(c)
		return
	}
	f := c.Request.PostForm
	keyword, value := f.Get("alias-keyword"), f.Get("alias-value")
	if keyword == "" || value == "" {
		serve200(c)
		return
	}
	rules := c.effectiveRules()
	rules.Aliases[keyword] = value
	if c.Config.App.UserHandling {
		if err := model.SaveUserRules(c.UserID, rules); err != nil {
			log.Error().Err(err).Msg("failed to save user rules")
			serve500(c)
			return
		}
		c.userRules = rules
	} else {
		if err := c.Config.SaveRules(); err != nil {
			log.Error().Err(err).Msg("failed to save rules")
			serve500(c)
			return
		}
	}
	serve200(c)
}

func serveDeleteAlias(c *webContext) {
	err := c.Request.ParseForm()
	if err != nil {
		serve500(c)
		return
	}
	a := c.Request.PostForm.Get("alias")
	rules := c.effectiveRules()
	if _, ok := rules.Aliases[a]; !ok {
		serve500(c)
		return
	}
	delete(rules.Aliases, a)
	if c.Config.App.UserHandling {
		if err := model.SaveUserRules(c.UserID, rules); err != nil {
			log.Error().Err(err).Msg("failed to save user rules")
			serve500(c)
			return
		}
		c.userRules = rules
	} else {
		if err := c.Config.SaveRules(); err != nil {
			log.Error().Err(err).Msg("failed to save rules")
			serve500(c)
			return
		}
	}
	serve200(c)
}

func serveDelete(c *webContext) {
	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		http.Error(c.Response, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Non-admin users may only delete their own documents.
	var userID *uint
	if c.Config.App.UserHandling && !c.IsAdmin {
		userID = &c.UserID
	}
	count, err := indexer.DeleteByQuery(req.Query, userID, func(url string, uid uint) {
		if err := model.DeleteHistoryURL(uid, url); err != nil {
			log.Warn().Err(err).Str("url", url).Msg("failed to delete history for deleted document")
		}
	})
	if err != nil {
		if errors.Is(err, indexer.ErrEmptyFilter) {
			http.Error(c.Response, err.Error(), http.StatusBadRequest)
			return
		}
		log.Error().Err(err).Msg("delete failed")
		serve500(c)
		return
	}
	c.JSON(map[string]any{"deleted": count})
}

type batchOp struct {
	Op      string `json:"op"`
	URL     string `json:"url"`
	Title   string `json:"title"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
	Favicon string `json:"favicon"`
}

type batchOpResult struct {
	Status   int                `json:"status"`
	Error    string             `json:"error,omitempty"`
	Document *document.Document `json:"document,omitempty"`
}

type batchRequest struct {
	Ops []batchOp `json:"ops"`
}

type batchResponse struct {
	Results []batchOpResult `json:"results,omitempty"`
	Error   string          `json:"error,omitempty"`
}

const (
	maxBatchOps = 100
	batchOpAdd  = "add"
	batchOpDel  = "delete"
	batchOpGet  = "get"
)

// TODO handle data dir updates
func serveBatch(c *webContext) {
	c.Request.Body = http.MaxBytesReader(c.Response, c.Request.Body, 5<<20) // 5 MB
	var req batchRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSONStatus(http.StatusBadRequest, batchResponse{Error: "invalid JSON"})
		return
	}

	if len(req.Ops) == 0 {
		c.JSONStatus(http.StatusBadRequest, batchResponse{Error: "empty batch"})
		return
	}

	if len(req.Ops) > maxBatchOps {
		c.JSONStatus(http.StatusBadRequest, batchResponse{Error: "too many operations (max 100)"})
		return
	}

	batch := indexer.NewMultiBatch()
	results := make([]batchOpResult, len(req.Ops))
	for i, op := range req.Ops {
		switch op.Op {
		case batchOpAdd:
			if op.URL == "" {
				results[i] = batchOpResult{Status: http.StatusBadRequest, Error: "missing url"}
				continue
			}
			d := &document.Document{URL: op.URL, Title: op.Title, Text: op.Text, HTML: op.HTML, Favicon: op.Favicon}
			if c.effectiveRules().IsSkip(d.URL) || strings.HasPrefix(d.URL, c.Config.BaseURL("/")) {
				results[i] = batchOpResult{Status: http.StatusNotAcceptable, Error: "url skipped by rules"}
				continue
			}
			if err := batch.Add(d); err != nil {
				if errors.Is(err, document.ErrSensitiveContent) {
					log.Warn().Str("URL", op.URL).Msg("rejected document: sensitive content")
					results[i] = batchOpResult{Status: http.StatusUnprocessableEntity, Error: document.ErrSensitiveContent.Error()}
				} else {
					log.Error().Err(err).Str("URL", op.URL).Msg("batch add error")
					results[i] = batchOpResult{Status: http.StatusInternalServerError, Error: "internal error"}
				}
			} else {
				results[i] = batchOpResult{Status: http.StatusCreated}
			}
		case batchOpDel:
			if op.URL == "" {
				results[i] = batchOpResult{Status: http.StatusBadRequest, Error: "missing url"}
				continue
			}
			batch.Delete(op.URL)
			results[i] = batchOpResult{Status: http.StatusOK}
		case batchOpGet:
			if op.URL == "" {
				results[i] = batchOpResult{Status: http.StatusBadRequest, Error: "missing url"}
				continue
			}
			d := indexer.GetByURLAndUser(op.URL, c.UserID)
			if d == nil {
				results[i] = batchOpResult{Status: http.StatusNotFound, Error: "document not found"}
			} else {
				results[i] = batchOpResult{Status: http.StatusOK, Document: d}
			}
		default:
			results[i] = batchOpResult{Status: http.StatusBadRequest, Error: fmt.Sprintf("unknown op: %q", op.Op)}
		}
	}

	if err := batch.Save(); err != nil {
		log.Error().Err(err).Msg("batch save error")
		c.JSONStatus(http.StatusInternalServerError, batchResponse{Error: "internal error"})
		return
	}

	log.Debug().Int("ops", len(req.Ops)).Msg("batch request processed")
	c.JSON(batchResponse{Results: results})
}

type reindexRequest struct {
	SkipSensitive   bool `json:"skipSensitive"`
	DetectLanguages bool `json:"detectLanguages"`
}

func serveReindex(c *webContext) {
	var req reindexRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		serve500(c)
		return
	}
	if err := indexer.Reindex(c.Config.FullPath(""), c.Config.Rules, req.SkipSensitive, req.DetectLanguages, c.Config.Indexer.Directories); err != nil {
		log.Error().Err(err).Msg("reindex failed")
		serve500(c)
		return
	}
	if err := model.SetIndexerVersion(indexer.Version); err != nil {
		log.Error().Err(err).Msg("failed to update indexer version")
		serve500(c)
		return
	}
	serve200(c)
}

func serveCleanup(c *webContext) {
	htmlRemoved, faviconRemoved, err := indexer.CleanupDataFiles(c.Config.FullPath(""))
	if err != nil {
		log.Error().Err(err).Msg("cleanup failed")
		serve500(c)
		return
	}
	c.JSON(map[string]int{
		"htmlRemoved":    htmlRemoved,
		"faviconRemoved": faviconRemoved,
	})
}

func serveFavicon(c *webContext) {
	i, err := iofs.ReadFile(appSubFS, "favicon.ico")
	if err != nil {
		serve500(c)
		return
	}
	c.Response.Header().Add("Content-Type", "image/vnd.microsoft.icon")
	if _, err := c.Response.Write(i); err != nil {
		log.Warn().Err(err).Msg("failed to write favicon response")
	}
}

func serveStatic(c *webContext) {
	staticFileServer.ServeHTTP(c.Response, c.Request)
}

func serve200(c *webContext) {
	c.Response.WriteHeader(http.StatusOK)
}

// uniqueStrings returns a copy of ss with duplicate entries removed,
// preserving the first occurrence of each value.
func uniqueStrings(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

func serve403(c *webContext) {
	c.Response.WriteHeader(http.StatusForbidden)
}

func serve500(c *webContext) {
	http.Error(c.Response, "Internal Server Error", http.StatusInternalServerError)
}

func (c *webContext) JSON(o any) {
	c.Response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(c.Response).Encode(o); err != nil {
		log.Error().Err(err).Msg("failed to encode JSON response")
	}
}

func (c *webContext) JSONStatus(status int, o any) {
	c.Response.Header().Set("Content-Type", "application/json")
	c.Response.WriteHeader(status)
	if err := json.NewEncoder(c.Response).Encode(o); err != nil {
		log.Error().Err(err).Msg("failed to encode JSON response")
	}
}

func (c *webContext) Redirect(u string) {
	http.Redirect(c.Response, c.Request, u, http.StatusFound)
}
