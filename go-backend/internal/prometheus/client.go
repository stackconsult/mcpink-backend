package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Config struct {
	QueryURL string
	Username string
	Password string
}

type Client struct {
	httpClient *http.Client
	config     Config
}

func NewClient(cfg Config) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		config:     cfg,
	}
}

type DataPoint struct {
	Timestamp time.Time
	Value     float64
}

type promResponse struct {
	Status string   `json:"status"`
	Data   promData `json:"data"`
}

type promData struct {
	ResultType string       `json:"resultType"`
	Result     []promResult `json:"result"`
}

type promResult struct {
	Metric map[string]string `json:"metric"`
	Values [][]any           `json:"values"`
}

func (c *Client) QueryRange(ctx context.Context, query, start, end, step string) ([]DataPoint, error) {
	params := url.Values{
		"query": {query},
		"start": {start},
		"end":   {end},
		"step":  {step},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.QueryURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if c.config.Username != "" {
		req.SetBasicAuth(c.config.Username, c.config.Password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying prometheus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus returned status %d", resp.StatusCode)
	}

	var promResp promResponse
	if err := json.NewDecoder(resp.Body).Decode(&promResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if promResp.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: status=%s", promResp.Status)
	}

	var points []DataPoint
	for _, result := range promResp.Data.Result {
		for _, pair := range result.Values {
			if len(pair) != 2 {
				continue
			}

			ts, ok := pair[0].(float64)
			if !ok {
				continue
			}

			valStr, ok := pair[1].(string)
			if !ok {
				continue
			}

			val, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				continue
			}

			points = append(points, DataPoint{
				Timestamp: time.Unix(int64(ts), 0),
				Value:     val,
			})
		}
	}

	if points == nil {
		points = []DataPoint{}
	}

	return points, nil
}
