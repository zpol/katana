package jfrog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/goxray/goxray/internal/metrics"
)

// ArtifactRef identifies an artifact in Artifactory / Xray.
type ArtifactRef struct {
	Repo     string `json:"repo,omitempty"`
	Path     string `json:"path,omitempty"`
	Sha      string `json:"sha,omitempty"`
	ImageRef string `json:"image_ref,omitempty"`
}

// ScanSummary is a normalized Xray scan result.
type ScanSummary struct {
	Artifact     ArtifactRef `json:"artifact"`
	Scanned      bool        `json:"scanned"`
	LookupStatus string      `json:"lookup_status,omitempty"` // indexed | not_indexed | unavailable
	Critical     int         `json:"critical"`
	High         int         `json:"high"`
	Medium       int         `json:"medium"`
	Low          int         `json:"low"`
	Unknown      int         `json:"unknown"`
	Violations   []string    `json:"violations,omitempty"`
	RawError     string      `json:"error,omitempty"`
}

// Status describes integration readiness (no secrets).
type Status struct {
	Configured bool   `json:"configured"`
	BaseURL    string `json:"base_url,omitempty"`
	Reachable  bool   `json:"reachable"`
	Message    string `json:"message"`
}

// Client is the JFrog Xray / Artifactory integration surface.
type Client interface {
	Configured() bool
	Status(ctx context.Context) Status
	Ping(ctx context.Context) error
	GetScanSummary(ctx context.Context, ref ArtifactRef) (*ScanSummary, error)
}

// HTTPClient talks to JFrog Platform Xray APIs.
type scanCacheEntry struct {
	at  time.Time
	sum *ScanSummary
}

type HTTPClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	cacheMu    sync.Mutex
	cache      map[string]scanCacheEntry
}

// NewHTTPClient builds a client. Empty baseURL or token yields an unconfigured client.
func NewHTTPClient(baseURL, token string) *HTTPClient {
	timeout := timeoutFromEnv()
	return &HTTPClient{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:      strings.TrimSpace(token),
		httpClient: newHTTPClient(timeout),
		cache:      map[string]scanCacheEntry{},
	}
}

func scanCacheKey(ref ArtifactRef) string {
	return strings.TrimSpace(ref.ImageRef) + "|" + strings.TrimSpace(ref.Sha)
}

func scanCacheTTL() time.Duration {
	return 5 * time.Minute
}

func (c *HTTPClient) cacheGet(ref ArtifactRef) *ScanSummary {
	if c == nil {
		return nil
	}
	key := scanCacheKey(ref)
	if key == "|" {
		return nil
	}
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	ent, ok := c.cache[key]
	if !ok || time.Since(ent.at) > scanCacheTTL() {
		return nil
	}
	return ent.sum
}

func (c *HTTPClient) cachePut(ref ArtifactRef, sum *ScanSummary) {
	if c == nil || sum == nil || sum.LookupStatus == "unavailable" {
		return
	}
	key := scanCacheKey(ref)
	if key == "|" {
		return
	}
	c.cacheMu.Lock()
	c.cache[key] = scanCacheEntry{at: time.Now(), sum: sum}
	c.cacheMu.Unlock()
}

func timeoutFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("JFROG_TIMEOUT"))
	if raw == "" {
		return 45 * time.Second
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 5*time.Second {
		return 45 * time.Second
	}
	return d
}

func lookupBatchTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("JFROG_LOOKUP_BATCH_TIMEOUT"))
	if raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d >= 3*time.Second {
			return d
		}
	}
	return 12 * time.Second
}

func newHTTPClient(timeout time.Duration) *http.Client {
	headerWait := timeout - 5*time.Second
	if headerWait < 15*time.Second {
		headerWait = timeout
	}
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   20 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: headerWait,
		ExpectContinueTimeout: 2 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		// HTTP/2 through corporate SSL inspection proxies often hangs
		// until Client.Timeout with "awaiting headers".
		ForceAttemptHTTP2: false,
	}
	if raw := strings.TrimSpace(os.Getenv("JFROG_PROXY")); raw != "" {
		if u, err := url.Parse(raw); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}

// Configured reports whether credentials are present.
func (c *HTTPClient) Configured() bool {
	return c != nil && c.baseURL != "" && c.token != ""
}

// Status returns a safe integration status (token never included).
func (c *HTTPClient) Status(ctx context.Context) Status {
	if !c.Configured() {
		return Status{Configured: false, Message: "JFROG_URL / JFROG_TOKEN not set"}
	}
	st := Status{Configured: true, BaseURL: c.baseURL}
	if err := c.Ping(ctx); err != nil {
		st.Reachable = false
		st.Message = err.Error()
		return st
	}
	st.Reachable = true
	st.Message = "ok"
	return st
}

// Ping hits Xray system ping.
func (c *HTTPClient) Ping(ctx context.Context) error {
	if !c.Configured() {
		return fmt.Errorf("jfrog not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/xray/api/v1/system/ping", nil)
	if err != nil {
		return err
	}
	c.auth(req)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return fmt.Errorf("ping status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

type summaryRequest struct {
	Paths     []string `json:"paths,omitempty"`
	Checksums []string `json:"checksums,omitempty"`
}

type summaryResponse struct {
	Artifacts []struct {
		General struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"general"`
		Issues []struct {
			Severity  string `json:"severity"`
			IssueType string `json:"issue_type"`
			Summary   string `json:"summary"`
			IssueID   string `json:"issue_id"`
			CVEs      []struct {
				CVE string `json:"cve"`
			} `json:"cves"`
		} `json:"issues"`
		Error string `json:"error"`
	} `json:"artifacts"`
	Errors []struct {
		Error string `json:"error"`
	} `json:"errors"`
}

// GetScanSummary queries Xray artifact summary for an image/path/checksum.
func (c *HTTPClient) GetScanSummary(ctx context.Context, ref ArtifactRef) (sum *ScanSummary, err error) {
	start := time.Now()
	defer func() {
		status := "unavailable"
		if sum != nil && sum.LookupStatus != "" {
			status = sum.LookupStatus
		}
		metrics.ObserveXrayLookup(status, time.Since(start))
	}()
	if !c.Configured() {
		return &ScanSummary{
			Artifact:     ref,
			Scanned:      false,
			LookupStatus: "unavailable",
			RawError:     "jfrog not configured",
			Violations: []string{
				"jfrog not configured: set JFROG_URL and JFROG_TOKEN",
			},
		}, nil
	}
	if cached := c.cacheGet(ref); cached != nil {
		return cached, nil
	}

	if ref.Sha != "" {
		out, done, err := c.lookupSummary(ctx, ref, summaryRequest{Checksums: []string{ref.Sha}})
		if err != nil {
			return unavailableSummary(ref, err.Error()), nil
		}
		if done && out != nil {
			c.cachePut(ref, out)
			return out, nil
		}
		return unavailableSummary(ref, "checksum lookup returned no result"), nil
	}

	paths := artifactPaths(ref)
	if len(paths) == 0 {
		return &ScanSummary{
			Artifact:     ref,
			Scanned:      false,
			LookupStatus: "unavailable",
			RawError:     "insufficient artifact coordinates",
			Violations:   []string{"need image path, repo/path, or checksum"},
		}, nil
	}

	batches := pathBatches(paths)
	var lastTransport error
	var lastNotIndexed *ScanSummary
	for _, batch := range batches {
		if err := ctx.Err(); err != nil {
			break
		}
		batchCtx, cancel := context.WithTimeout(ctx, lookupBatchTimeout())
		out, done, err := c.lookupSummary(batchCtx, ref, summaryRequest{Paths: batch})
		cancel()
		if err != nil {
			lastTransport = err
			continue
		}
		if !done || out == nil {
			continue
		}
		if out.LookupStatus == "indexed" {
			c.cachePut(ref, out)
			return out, nil
		}
		if out.LookupStatus == "not_indexed" {
			lastNotIndexed = out
			continue
		}
		c.cachePut(ref, out)
		return out, nil
	}
	if lastNotIndexed != nil {
		c.cachePut(ref, lastNotIndexed)
		return lastNotIndexed, nil
	}

	msg := "xray lookup timed out or failed"
	if lastTransport != nil {
		msg = lastTransport.Error()
	}
	return unavailableSummary(ref, msg), nil
}

func (c *HTTPClient) lookupSummary(ctx context.Context, ref ArtifactRef, payload summaryRequest) (*ScanSummary, bool, error) {
	raw, status, err := c.postJSON(ctx, "/xray/api/v1/summary/artifact", payload)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		raw, status, err = c.postJSON(ctx, "/xray/api/v2/summary/artifact", payload)
		if err != nil {
			return nil, false, err
		}
	}
	if status >= 300 {
		out := &ScanSummary{
			Artifact:     ref,
			Scanned:      false,
			LookupStatus: "unavailable",
			RawError:     fmt.Sprintf("xray summary status %d: %s", status, truncate(string(raw), 300)),
			Violations:   []string{fmt.Sprintf("xray summary failed (%d)", status)},
		}
		return out, true, nil
	}

	out, done := parseSummaryResponse(ref, raw)
	return out, done, nil
}

func parseSummaryResponse(ref ArtifactRef, raw []byte) (*ScanSummary, bool) {
	var parsed summaryResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, false
	}

	out := &ScanSummary{Artifact: ref, Scanned: false, LookupStatus: "not_indexed"}
	if len(parsed.Artifacts) == 0 {
		if len(parsed.Errors) > 0 {
			out.RawError = enrichXrayError(parsed.Errors[0].Error)
			out.Violations = append(out.Violations, out.RawError)
		} else {
			out.Violations = append(out.Violations, "no artifact summary returned (unscanned or unknown)")
		}
		return out, true
	}

	art := parsed.Artifacts[0]
	if art.Error != "" {
		out.RawError = enrichXrayError(art.Error)
		out.Violations = append(out.Violations, out.RawError)
		return out, true
	}
	out.Scanned = true
	out.LookupStatus = "indexed"
	for _, issue := range art.Issues {
		if !strings.EqualFold(issue.IssueType, "security") && issue.IssueType != "" {
			continue
		}
		sev := strings.ToLower(issue.Severity)
		switch sev {
		case "critical":
			out.Critical++
		case "high":
			out.High++
		case "medium":
			out.Medium++
		case "low":
			out.Low++
		default:
			out.Unknown++
		}
		label := issue.IssueID
		if label == "" && len(issue.CVEs) > 0 {
			label = issue.CVEs[0].CVE
		}
		if label == "" {
			label = issue.Summary
		}
		if label != "" && len(out.Violations) < 25 {
			out.Violations = append(out.Violations, fmt.Sprintf("%s (%s)", label, sev))
		}
	}
	return out, true
}

func unavailableSummary(ref ArtifactRef, msg string) *ScanSummary {
	msg = strings.TrimSpace(msg)
	return &ScanSummary{
		Artifact:     ref,
		Scanned:      false,
		LookupStatus: "unavailable",
		RawError:     msg,
		Violations:   []string{msg},
	}
}

func pathBatches(paths []string) [][]string {
	const batchSize = 4
	var batches [][]string
	for i := 0; i < len(paths); i += batchSize {
		end := i + batchSize
		if end > len(paths) {
			end = len(paths)
		}
		batches = append(batches, paths[i:end])
	}
	return batches
}

func (c *HTTPClient) postJSON(ctx context.Context, path string, body interface{}) ([]byte, int, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, 0, err
	}
	c.auth(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, res.StatusCode, err
	}
	return raw, res.StatusCode, nil
}

func (c *HTTPClient) auth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
}

func scanTransportError(ref ArtifactRef, err error) *ScanSummary {
	msg := err.Error()
	return &ScanSummary{
		Artifact:     ref,
		Scanned:      false,
		LookupStatus: "unavailable",
		RawError:     msg,
		Violations:   []string{msg},
	}
}

func artifactPaths(ref ArtifactRef) []string {
	var out []string
	if ref.Path != "" {
		if ref.Repo != "" {
			out = append(out, strings.Trim(ref.Repo, "/")+"/"+strings.TrimPrefix(ref.Path, "/"))
		} else {
			out = append(out, strings.TrimPrefix(ref.Path, "/"))
		}
	}
	if ref.ImageRef != "" {
		out = append(out, dockerManifestPaths(ref.ImageRef)...)
	}
	return unique(out)
}

// dockerManifestPaths guesses Artifactory docker layout paths for an image reference.
// Paths are ordered most-likely-first for faster Xray lookups.
func dockerManifestPaths(image string) []string {
	image = strings.TrimSpace(image)
	if image == "" {
		return nil
	}
	if i := strings.Index(image, "@"); i >= 0 {
		image = image[:i]
	}
	_, remainder := splitHost(image)
	name, tag := splitNameTag(remainder)
	if name == "" {
		return nil
	}
	if tag == "" {
		tag = "latest"
	}
	repo := firstPathSeg(name)
	rest := trimFirstSeg(name)

	var bases []string
	if repo != "" && rest != "" {
		bases = append(bases, fmt.Sprintf("%s/%s/%s", repo, rest, tag))
	}
	if prefix := xrayProjectPrefix(repo); prefix != "" && rest != "" {
		bases = append(bases, fmt.Sprintf("%s/%s/%s", prefix, rest, tag))
	}
	bases = append(bases, fmt.Sprintf("%s/%s", name, tag))
	bases = unique(bases)

	var ordered []string
	addFiles := func(base string, out *[]string) {
		if base == "" {
			return
		}
		for _, file := range []string{"manifest.json", "list.manifest.json"} {
			*out = append(*out, fmt.Sprintf("%s/%s", base, file))
		}
	}
	for _, b := range bases {
		addFiles(b, &ordered)
	}
	var fallback []string
	for _, b := range bases {
		addFiles("default/"+b, &fallback)
	}
	return unique(append(ordered, fallback...))
}

func splitHost(image string) (host, remainder string) {
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 1 {
		return "", image
	}
	if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" {
		return parts[0], parts[1]
	}
	return "", image
}

func splitNameTag(s string) (name, tag string) {
	if i := strings.LastIndex(s, ":"); i >= 0 {
		// avoid port-like in host already stripped
		return s[:i], s[i+1:]
	}
	return s, "latest"
}

func firstPathSeg(s string) string {
	if i := strings.Index(s, "/"); i >= 0 {
		return s[:i]
	}
	return s
}

func trimFirstSeg(s string) string {
	if i := strings.Index(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// xrayProjectPrefix maps Artifactory docker repo names to Xray path prefixes.
// e.g. myproj-docker-prod-local -> myproj (Xray indexes under project slug, not full repo name).
func xrayProjectPrefix(repo string) string {
	const marker = "-docker-"
	i := strings.Index(repo, marker)
	if i <= 0 {
		return ""
	}
	project := repo[:i]
	switch project {
	case "docker", "k8s", "global":
		return ""
	}
	return project
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func enrichXrayError(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return msg
	}
	if strings.Contains(strings.ToLower(msg), "not indexed") ||
		strings.Contains(strings.ToLower(msg), "doesn't exist") {
		return msg + " — image may exist in Artifactory but has no Xray scan/index entry (check repo indexing or re-scan in JFrog UI)"
	}
	return msg
}

// NormalizeBaseURL validates and returns a cleaned base URL.
func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("JFROG_URL must be http(s)")
	}
	return strings.TrimRight(u.String(), "/"), nil
}
