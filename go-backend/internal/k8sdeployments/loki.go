package k8sdeployments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

type LokiLogger struct {
	pushURL   string
	labels    map[string]string
	client    *http.Client
	buffer    [][]string
	batchSize int
}

func NewLokiLogger(pushURL string, labels map[string]string) *LokiLogger {
	return &LokiLogger{
		pushURL:   pushURL,
		labels:    labels,
		client:    &http.Client{Timeout: 5 * time.Second},
		batchSize: 50,
	}
}

func (l *LokiLogger) Log(line string) {
	ts := strconv.FormatInt(time.Now().UnixNano(), 10)
	l.buffer = append(l.buffer, []string{ts, line})
	if len(l.buffer) >= l.batchSize {
		_ = l.Flush(context.Background())
	}
}

func (l *LokiLogger) Flush(ctx context.Context) error {
	if len(l.buffer) == 0 || l.pushURL == "" {
		return nil
	}

	req := lokiPushRequest{
		Streams: []lokiStream{
			{
				Stream: l.labels,
				Values: l.buffer,
			},
		},
	}
	l.buffer = nil

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal loki push: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, l.pushURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create loki request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("loki push: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("loki push returned %d", resp.StatusCode)
	}
	return nil
}

// LokiQueryResult represents the response from Loki's query_range API.
type LokiQueryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// QueryLoki retrieves logs from Loki for a given LogQL query within a time range.
// queryURL should be the Loki query_range endpoint, e.g. http://loki:3100/loki/api/v1/query_range
func QueryLoki(ctx context.Context, queryURL, username, password, logQL string, start, end time.Time, limit int) (*LokiQueryResult, error) {
	if queryURL == "" {
		return nil, fmt.Errorf("loki query URL is empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create loki query request: %w", err)
	}
	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	q := req.URL.Query()
	q.Set("query", logQL)
	q.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	q.Set("direction", "forward")
	req.URL.RawQuery = q.Encode()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("loki query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("loki query returned %d", resp.StatusCode)
	}

	var result LokiQueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode loki response: %w", err)
	}
	return &result, nil
}

// QueryBuildLogs retrieves build logs for a specific service from Loki.
func QueryBuildLogs(ctx context.Context, lokiQueryURL, username, password, namespace, service string, since time.Duration, limit int) ([]string, error) {
	logQL := fmt.Sprintf(`{job="build", namespace=%q, service=%q}`, namespace, service)
	end := time.Now()
	start := end.Add(-since)

	result, err := QueryLoki(ctx, lokiQueryURL, username, password, logQL, start, end, limit)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, stream := range result.Data.Result {
		for _, entry := range stream.Values {
			if len(entry) >= 2 {
				lines = append(lines, entry[1])
			}
		}
	}
	return lines, nil
}

// QueryRunLogs retrieves runtime logs for a specific service from Loki.
func QueryRunLogs(ctx context.Context, lokiQueryURL, username, password, namespace, service string, since time.Duration, limit int) ([]string, error) {
	logQL := fmt.Sprintf(`{namespace=%q, container=%q}`, namespace, service)
	end := time.Now()
	start := end.Add(-since)

	result, err := QueryLoki(ctx, lokiQueryURL, username, password, logQL, start, end, limit)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, stream := range result.Data.Result {
		for _, entry := range stream.Values {
			if len(entry) >= 2 {
				lines = append(lines, entry[1])
			}
		}
	}
	return lines, nil
}
