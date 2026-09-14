// Dummy JFrog Xray for KATANA demos. Implements ping + summary/artifact only.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type catalogFile struct {
	Entries []catalogEntry `yaml:"entries"`
}

type catalogEntry struct {
	Match      string        `yaml:"match"`
	Indexed    bool          `yaml:"indexed"`
	HTTPStatus int           `yaml:"http_status"`
	Issues     []catalogIssue `yaml:"issues"`
}

type catalogIssue struct {
	Severity string `yaml:"severity"`
	IssueID  string `yaml:"issue_id"`
	Summary  string `yaml:"summary"`
}

type summaryRequest struct {
	Paths     []string `json:"paths"`
	Checksums []string `json:"checksums"`
}

type issueOut struct {
	Severity  string `json:"severity"`
	IssueType string `json:"issue_type"`
	IssueID   string `json:"issue_id"`
	Summary   string `json:"summary"`
	CVEs      []struct {
		CVE string `json:"cve"`
	} `json:"cves,omitempty"`
}

type artifactOut struct {
	General struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"general"`
	Issues []issueOut `json:"issues"`
	Error  string     `json:"error,omitempty"`
}

type summaryResponse struct {
	Artifacts []artifactOut `json:"artifacts"`
	Errors    []struct {
		Error string `json:"error"`
	} `json:"errors,omitempty"`
}

func main() {
	addr := flag.String("addr", envOr("DUMMY_JFROG_ADDR", ":8081"), "listen address")
	catalogPath := flag.String("catalog", envOr("DUMMY_JFROG_CATALOG", "catalog.yaml"), "catalog YAML path")
	requireAuth := flag.Bool("require-auth", true, "require Authorization Bearer token")
	flag.Parse()

	cat, err := loadCatalog(*catalogPath)
	if err != nil {
		log.Fatalf("catalog: %v", err)
	}
	log.Printf("dummy-jfrog listening on %s (catalog=%s, %d entries)", *addr, *catalogPath, len(cat.Entries))

	var mu sync.RWMutex
	mux := http.NewServeMux()
	mux.HandleFunc("/xray/api/v1/system/ping", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r, *requireAuth) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"OK"}`))
	})
	mux.HandleFunc("/xray/api/v1/summary/artifact", func(w http.ResponseWriter, r *http.Request) {
		handleSummary(w, r, &mu, cat, *requireAuth)
	})
	mux.HandleFunc("/xray/api/v2/summary/artifact", func(w http.ResponseWriter, r *http.Request) {
		handleSummary(w, r, &mu, cat, *requireAuth)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func handleSummary(w http.ResponseWriter, r *http.Request, mu *sync.RWMutex, cat *catalogFile, requireAuth bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !checkAuth(w, r, requireAuth) {
		return
	}
	var req summaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	keys := append(append([]string{}, req.Paths...), req.Checksums...)
	mu.RLock()
	entry, matchedPath := matchEntry(cat, keys)
	mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if entry != nil && entry.HTTPStatus >= 400 {
		w.WriteHeader(entry.HTTPStatus)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "simulated xray unavailable"})
		return
	}
	if entry == nil || !entry.Indexed {
		_ = json.NewEncoder(w).Encode(summaryResponse{Artifacts: nil})
		return
	}

	path := matchedPath
	if path == "" && len(keys) > 0 {
		path = keys[0]
	}
	art := artifactOut{Issues: make([]issueOut, 0, len(entry.Issues))}
	art.General.Name = filepath.Base(path)
	art.General.Path = path
	for _, iss := range entry.Issues {
		out := issueOut{
			Severity:  iss.Severity,
			IssueType: "security",
			IssueID:   iss.IssueID,
			Summary:   iss.Summary,
		}
		if strings.HasPrefix(strings.ToUpper(iss.IssueID), "CVE-") {
			out.CVEs = []struct {
				CVE string `json:"cve"`
			}{{CVE: iss.IssueID}}
		}
		art.Issues = append(art.Issues, out)
	}
	_ = json.NewEncoder(w).Encode(summaryResponse{Artifacts: []artifactOut{art}})
}

func matchEntry(cat *catalogFile, keys []string) (*catalogEntry, string) {
	for _, key := range keys {
		kl := strings.ToLower(key)
		for i := range cat.Entries {
			m := strings.ToLower(strings.TrimSpace(cat.Entries[i].Match))
			if m == "" {
				continue
			}
			if strings.Contains(kl, m) {
				return &cat.Entries[i], key
			}
		}
	}
	return nil, ""
}

func checkAuth(w http.ResponseWriter, r *http.Request, require bool) bool {
	if !require {
		return true
	}
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")) == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func loadCatalog(path string) (*catalogFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		// Try next to the binary / cwd fallbacks for container layout.
		alt := filepath.Join("/catalog", filepath.Base(path))
		b, err = os.ReadFile(alt)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		path = alt
	}
	var cat catalogFile
	if err := yaml.Unmarshal(b, &cat); err != nil {
		return nil, err
	}
	if len(cat.Entries) == 0 {
		return nil, fmt.Errorf("no entries in %s", path)
	}
	return &cat, nil
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
