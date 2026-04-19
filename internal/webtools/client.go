package webtools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	defaultSearchLimit    = 5
	maxSearchLimit        = 10
	defaultPageMaxChars   = 12000
	maxPageMaxChars       = 20000
	maxHTTPResponseBytes  = 512 * 1024
	defaultRequestTimeout = 25 * time.Second
	defaultHTTPUserAgent  = "Emerald Web Tools/1.0"
	jinaTitlePrefix       = "Title:"
	jinaMarkdownPrefix    = "Markdown Content:"
)

type Client struct {
	httpClient *http.Client
}

type SearchRequest struct {
	Query string
	Limit int
}

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
	Engine  string `json:"engine,omitempty"`
}

type SearchResponse struct {
	Provider    SearchProvider `json:"provider"`
	Query       string         `json:"query"`
	ResultCount int            `json:"result_count,omitempty"`
	Results     []SearchResult `json:"results,omitempty"`
	Content     string         `json:"content,omitempty"`
	Truncated   bool           `json:"truncated,omitempty"`
}

type OpenPageRequest struct {
	URL           string
	Mode          PageObservationMode
	MaxCharacters int
}

type OpenPageResponse struct {
	URL         string              `json:"url"`
	FinalURL    string              `json:"final_url,omitempty"`
	Mode        PageObservationMode `json:"mode"`
	StatusCode  int                 `json:"status_code,omitempty"`
	ContentType string              `json:"content_type,omitempty"`
	Title       string              `json:"title,omitempty"`
	Content     string              `json:"content"`
	Truncated   bool                `json:"truncated,omitempty"`
}

type searxngResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
		Engine  string `json:"engine"`
	} `json:"results"`
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultRequestTimeout}
	}
	return &Client{httpClient: httpClient}
}

func (c *Client) Search(ctx context.Context, cfg RuntimeConfig, req SearchRequest) (*SearchResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	limit := clampSearchLimit(req.Limit)

	switch cfg.SearchProvider {
	case SearchProviderSearXNG:
		return c.searchSearXNG(ctx, cfg, query, limit)
	case SearchProviderJina:
		return c.searchJina(ctx, cfg, query)
	default:
		return nil, fmt.Errorf("web search is not configured")
	}
}

func (c *Client) OpenPage(ctx context.Context, cfg RuntimeConfig, req OpenPageRequest) (*OpenPageResponse, error) {
	targetURL, err := normalizeTargetURL(req.URL)
	if err != nil {
		return nil, err
	}

	mode := req.Mode
	if mode == "" {
		mode = cfg.PageObservationMode
	}
	if mode == "" {
		mode = PageObservationModeHTTP
	}

	maxCharacters := clampPageMaxCharacters(req.MaxCharacters)
	switch mode {
	case PageObservationModeJina:
		return c.openPageWithJina(ctx, cfg, targetURL, maxCharacters)
	case PageObservationModeHTTP:
		return c.openPageWithHTTP(ctx, targetURL, maxCharacters)
	default:
		return nil, fmt.Errorf("unsupported page observation mode %q", mode)
	}
}

func (c *Client) searchSearXNG(ctx context.Context, cfg RuntimeConfig, query string, limit int) (*SearchResponse, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.SearXNGBaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("SearXNG base URL is not configured")
	}

	endpoint, err := buildSearXNGSearchURL(baseURL, query)
	if err != nil {
		return nil, err
	}

	responseBody, _, _, _, err := c.doRequest(ctx, endpoint, "", nil)
	if err != nil {
		return nil, err
	}

	var payload searxngResponse
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return nil, fmt.Errorf("decode SearXNG search response: %w", err)
	}

	results := make([]SearchResult, 0, min(limit, len(payload.Results)))
	for _, result := range payload.Results {
		if len(results) >= limit {
			break
		}
		if strings.TrimSpace(result.URL) == "" {
			continue
		}
		results = append(results, SearchResult{
			Title:   strings.TrimSpace(result.Title),
			URL:     strings.TrimSpace(result.URL),
			Snippet: truncateString(strings.TrimSpace(result.Content), 420),
			Engine:  strings.TrimSpace(result.Engine),
		})
	}

	return &SearchResponse{
		Provider:    SearchProviderSearXNG,
		Query:       query,
		ResultCount: len(results),
		Results:     results,
	}, nil
}

func (c *Client) searchJina(ctx context.Context, cfg RuntimeConfig, query string) (*SearchResponse, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.JinaSearchBaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("jina search base URL is not configured")
	}
	if strings.TrimSpace(cfg.JinaAPIKey) == "" {
		return nil, fmt.Errorf("jina search requires an API key")
	}

	endpoint := baseURL + "/?q=" + url.QueryEscape(query)
	headers := map[string]string{
		"Authorization": "Bearer " + cfg.JinaAPIKey,
		"Accept":        "text/plain",
	}

	responseBody, _, _, _, err := c.doRequest(ctx, endpoint, "", headers)
	if err != nil {
		return nil, err
	}

	content, truncated := truncateStringWithFlag(strings.TrimSpace(string(responseBody)), 8000)
	return &SearchResponse{
		Provider:  SearchProviderJina,
		Query:     query,
		Content:   content,
		Truncated: truncated,
	}, nil
}

func (c *Client) openPageWithHTTP(ctx context.Context, targetURL string, maxCharacters int) (*OpenPageResponse, error) {
	responseBody, finalURL, contentType, statusCode, err := c.doRequest(ctx, targetURL, "", map[string]string{
		"Accept":     "text/html,application/xhtml+xml,application/xml;q=0.9,text/plain,application/json;q=0.8,*/*;q=0.7",
		"User-Agent": defaultHTTPUserAgent,
	})
	if err != nil {
		return nil, err
	}

	title, content := extractReadableContent(contentType, responseBody)
	content, truncated := truncateStringWithFlag(content, maxCharacters)

	return &OpenPageResponse{
		URL:         targetURL,
		FinalURL:    finalURL,
		Mode:        PageObservationModeHTTP,
		StatusCode:  statusCode,
		ContentType: contentType,
		Title:       title,
		Content:     content,
		Truncated:   truncated,
	}, nil
}

func (c *Client) openPageWithJina(ctx context.Context, cfg RuntimeConfig, targetURL string, maxCharacters int) (*OpenPageResponse, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.JinaReaderBaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("jina reader base URL is not configured")
	}

	readerURL := baseURL + "/" + encodeJinaTargetURL(targetURL)
	headers := map[string]string{
		"Accept":     "text/plain",
		"User-Agent": defaultHTTPUserAgent,
	}
	if strings.TrimSpace(cfg.JinaAPIKey) != "" {
		headers["Authorization"] = "Bearer " + cfg.JinaAPIKey
	}

	responseBody, _, contentType, statusCode, err := c.doRequest(ctx, readerURL, "", headers)
	if err != nil {
		return nil, err
	}

	title, content := extractJinaReaderContent(string(responseBody))
	content, truncated := truncateStringWithFlag(content, maxCharacters)

	return &OpenPageResponse{
		URL:         targetURL,
		FinalURL:    readerURL,
		Mode:        PageObservationModeJina,
		StatusCode:  statusCode,
		ContentType: contentType,
		Title:       title,
		Content:     content,
		Truncated:   truncated,
	}, nil
}

func (c *Client) doRequest(ctx context.Context, endpoint string, method string, headers map[string]string) ([]byte, string, string, int, error) {
	httpMethod := strings.TrimSpace(method)
	if httpMethod == "" {
		httpMethod = http.MethodGet
	}

	request, err := http.NewRequestWithContext(ctx, httpMethod, endpoint, nil)
	if err != nil {
		return nil, "", "", 0, fmt.Errorf("build request: %w", err)
	}
	for key, value := range headers {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		request.Header.Set(key, value)
	}
	if request.Header.Get("User-Agent") == "" {
		request.Header.Set("User-Agent", defaultHTTPUserAgent)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, "", "", 0, fmt.Errorf("perform request: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxHTTPResponseBytes))
	if err != nil {
		return nil, "", "", response.StatusCode, fmt.Errorf("read response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", "", response.StatusCode, fmt.Errorf("remote request failed with status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	finalURL := endpoint
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL.String()
	}

	return body, finalURL, response.Header.Get("Content-Type"), response.StatusCode, nil
}

func buildSearXNGSearchURL(baseURL string, query string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid SearXNG base URL: %w", err)
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch path {
	case "", "/":
		parsed.Path = "/search"
	case "/search":
		parsed.Path = "/search"
	default:
		if strings.HasSuffix(path, "/search") {
			parsed.Path = path
		} else {
			parsed.Path = path + "/search"
		}
	}

	values := parsed.Query()
	values.Set("q", query)
	values.Set("format", "json")
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func normalizeTargetURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("url is required")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("URL must use http or https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("URL must include a host")
	}
	return parsed.String(), nil
}

func encodeJinaTargetURL(target string) string {
	escaped := url.PathEscape(target)
	replacer := strings.NewReplacer(
		"%3A", ":",
		"%2F", "/",
	)
	return replacer.Replace(escaped)
}

func extractReadableContent(contentType string, body []byte) (string, string) {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return "", ""
	}

	if strings.Contains(strings.ToLower(contentType), "html") || looksLikeHTML(raw) {
		title, content := extractHTMLContent(raw)
		if content != "" {
			return title, content
		}
	}

	return "", collapseWhitespace(raw)
}

func extractHTMLContent(raw string) (string, string) {
	document, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return "", collapseWhitespace(raw)
	}

	var title string
	var titleWalker func(*html.Node)
	titleWalker = func(node *html.Node) {
		if node == nil || title != "" {
			return
		}
		if node.Type == html.ElementNode && strings.EqualFold(node.Data, "title") {
			title = collapseWhitespace(nodeText(node))
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			titleWalker(child)
		}
	}
	titleWalker(document)

	bodyNode := findHTMLNode(document, "body")
	if bodyNode == nil {
		bodyNode = document
	}

	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}
		if node.Type == html.ElementNode {
			switch strings.ToLower(node.Data) {
			case "script", "style", "noscript", "svg", "canvas", "template":
				return
			}
		}
		if node.Type == html.TextNode {
			text := collapseWhitespace(html.UnescapeString(node.Data))
			if text != "" {
				if builder.Len() > 0 {
					builder.WriteByte('\n')
				}
				builder.WriteString(text)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(bodyNode)

	return title, collapseWhitespace(builder.String())
}

func extractJinaReaderContent(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ""
	}

	title := ""
	content := trimmed

	lines := strings.Split(trimmed, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, jinaTitlePrefix) {
			title = strings.TrimSpace(strings.TrimPrefix(line, jinaTitlePrefix))
			break
		}
	}

	if index := strings.Index(trimmed, jinaMarkdownPrefix); index >= 0 {
		content = strings.TrimSpace(trimmed[index+len(jinaMarkdownPrefix):])
	}

	return title, content
}

func findHTMLNode(node *html.Node, name string) *html.Node {
	if node == nil {
		return nil
	}
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, name) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLNode(child, name); found != nil {
			return found
		}
	}
	return nil
}

func nodeText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current == nil {
			return
		}
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return builder.String()
}

func looksLikeHTML(raw string) bool {
	preview := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(preview, "<!doctype html") || strings.HasPrefix(preview, "<html") || strings.Contains(preview, "<body")
}

func collapseWhitespace(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	fields := strings.Fields(value)
	return strings.TrimSpace(strings.Join(fields, " "))
}

func clampSearchLimit(value int) int {
	if value <= 0 {
		return defaultSearchLimit
	}
	if value > maxSearchLimit {
		return maxSearchLimit
	}
	return value
}

func clampPageMaxCharacters(value int) int {
	if value <= 0 {
		return defaultPageMaxChars
	}
	if value > maxPageMaxChars {
		return maxPageMaxChars
	}
	return value
}

func truncateString(value string, limit int) string {
	truncated, _ := truncateStringWithFlag(value, limit)
	return truncated
}

func truncateStringWithFlag(value string, limit int) (string, bool) {
	if limit <= 0 {
		return value, false
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes), false
	}
	return strings.TrimSpace(string(runes[:limit])) + "…", true
}

func min(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
