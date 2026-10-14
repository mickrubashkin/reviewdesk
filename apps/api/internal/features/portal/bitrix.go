package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client calls the Bitrix24 REST API through an inbound webhook. It is used
// read-only and paces requests to stay under the portal rate limit.
type Client struct {
	webhookURL string
	http       *http.Client
	minGap     time.Duration
	lastCall   time.Time
}

var ErrNotConfigured = errors.New("BITRIX_WEBHOOK_URL is not configured")

func NewClientFromEnv() (*Client, error) {
	webhookURL := strings.TrimSpace(os.Getenv("BITRIX_WEBHOOK_URL"))
	if webhookURL == "" {
		return nil, ErrNotConfigured
	}
	return &Client{
		webhookURL: strings.TrimRight(webhookURL, "/"),
		http:       &http.Client{Timeout: 60 * time.Second},
		minGap:     550 * time.Millisecond,
	}, nil
}

type Response struct {
	Result           json.RawMessage `json:"result"`
	Next             *int            `json:"next"`
	Total            int             `json:"total"`
	Error            string          `json:"error"`
	ErrorDescription string          `json:"error_description"`
}

// Call runs one REST method. Errors never include the webhook URL, which is
// a secret.
func (c *Client) Call(ctx context.Context, method string, params any) (Response, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return Response{}, err
	}

	for attempt := 0; ; attempt++ {
		if wait := c.minGap - time.Since(c.lastCall); wait > 0 {
			select {
			case <-ctx.Done():
				return Response{}, ctx.Err()
			case <-time.After(wait):
			}
		}
		c.lastCall = time.Now()

		request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL+"/"+method+".json", bytes.NewReader(body))
		if err != nil {
			return Response{}, fmt.Errorf("%s: build request", method)
		}
		request.Header.Set("Content-Type", "application/json")

		response, err := c.http.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return Response{}, ctx.Err()
			}
			return Response{}, fmt.Errorf("%s: request failed", method)
		}
		var decoded Response
		decodeErr := json.NewDecoder(response.Body).Decode(&decoded)
		response.Body.Close()

		if decoded.Error == "QUERY_LIMIT_EXCEEDED" && attempt < 5 {
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		if decoded.Error != "" {
			return decoded, fmt.Errorf("%s: %s %s", method, decoded.Error, decoded.ErrorDescription)
		}
		if decodeErr != nil {
			return Response{}, fmt.Errorf("%s: unexpected response (HTTP %d)", method, response.StatusCode)
		}
		return decoded, nil
	}
}

// List pages through a list method with the classic start/next pagination,
// passing each page's raw result to fn.
func (c *Client) List(ctx context.Context, method string, params map[string]any, fn func(json.RawMessage) error) error {
	start := 0
	for {
		params["start"] = start
		response, err := c.Call(ctx, method, params)
		if err != nil {
			return err
		}
		if err := fn(response.Result); err != nil {
			return err
		}
		if response.Next == nil {
			return nil
		}
		start = *response.Next
	}
}

type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (c *Client) DealCategories(ctx context.Context) ([]Category, error) {
	response, err := c.Call(ctx, "crm.category.list", map[string]any{"entityTypeId": 2})
	if err != nil {
		return nil, err
	}
	var result struct {
		Categories []Category `json:"categories"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, err
	}
	return result.Categories, nil
}

type Stage struct {
	ID   string
	Name string
	Sort int
}

func (c *Client) DealStages(ctx context.Context, categoryID int) ([]Stage, error) {
	entityID := "DEAL_STAGE"
	if categoryID != 0 {
		entityID = fmt.Sprintf("DEAL_STAGE_%d", categoryID)
	}
	response, err := c.Call(ctx, "crm.status.list", map[string]any{
		"filter": map[string]any{"ENTITY_ID": entityID},
		"order":  map[string]any{"SORT": "ASC"},
	})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		StatusID string `json:"STATUS_ID"`
		Name     string `json:"NAME"`
		Sort     string `json:"SORT"`
	}
	if err := json.Unmarshal(response.Result, &rows); err != nil {
		return nil, err
	}
	stages := make([]Stage, 0, len(rows))
	for _, row := range rows {
		var sort int
		fmt.Sscan(row.Sort, &sort)
		stages = append(stages, Stage{ID: row.StatusID, Name: row.Name, Sort: sort})
	}
	return stages, nil
}
