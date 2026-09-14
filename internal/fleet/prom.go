package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type promClient struct {
	base   string
	client *http.Client
}

func newPromClient(base string) *promClient {
	return &promClient{
		base: strings.TrimRight(strings.TrimSpace(base), "/"),
		client: &http.Client{
			Timeout: 12 * time.Second,
		},
	}
}

type promResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string       `json:"resultType"`
		Result     []promSample `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}

type promSample struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
}

func (c *promClient) query(ctx context.Context, q string) ([]promSample, error) {
	if c.base == "" {
		return nil, fmt.Errorf("PROMETHEUS_URL is empty")
	}
	u := c.base + "/api/v1/query?query=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("prometheus %d: %s", res.StatusCode, truncate(string(raw), 200))
	}
	var out promResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out.Status != "success" {
		msg := out.Error
		if msg == "" {
			msg = "query failed"
		}
		return nil, fmt.Errorf("prometheus: %s", msg)
	}
	return out.Data.Result, nil
}

func sampleValue(s promSample) float64 {
	if len(s.Value) < 2 {
		return 0
	}
	switch v := s.Value[1].(type) {
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	case float64:
		return v
	default:
		f, _ := strconv.ParseFloat(fmt.Sprint(v), 64)
		return f
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
