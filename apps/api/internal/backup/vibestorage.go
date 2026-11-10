package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const apiBase = "https://vibecode.bitrix24.tech/v1"

// VibeStorage is the platform's object storage (scope vibe:storage). Files
// are private to the key's owner.
type VibeStorage struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

// VibeStorageFromEnv uses the platform key BITRIX_API_KEY and the optional
// BITRIX_API_BASE_URL.
func VibeStorageFromEnv() (*VibeStorage, error) {
	key := strings.TrimSpace(os.Getenv("BITRIX_API_KEY"))
	if key == "" {
		return nil, errors.New("BITRIX_API_KEY is not set")
	}
	return &VibeStorage{
		APIKey:  key,
		BaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("BITRIX_API_BASE_URL")), "/"),
	}, nil
}

func (s *VibeStorage) Put(ctx context.Context, key string, body []byte) error {
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	if err := writer.WriteField("key", key); err != nil {
		return err
	}
	file, err := writer.CreateFormFile("file", key[strings.LastIndex(key, "/")+1:])
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	_, err = s.do(ctx, http.MethodPost, "/storage/objects/upload", writer.FormDataContentType(), &form)
	return err
}

func (s *VibeStorage) Keys(ctx context.Context, prefix string) ([]string, error) {
	keys := []string{}
	cursor := ""
	for {
		query := url.Values{"prefix": {prefix}, "limit": {"500"}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		body, err := s.do(ctx, http.MethodGet, "/storage/objects?"+query.Encode(), "", nil)
		if err != nil {
			return nil, err
		}
		var page struct {
			Data []struct {
				Key string `json:"key"`
			} `json:"data"`
			Cursor *string `json:"cursor"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("vibe storage: list: %w", err)
		}
		for _, object := range page.Data {
			keys = append(keys, object.Key)
		}
		if page.Cursor == nil || *page.Cursor == "" {
			return keys, nil
		}
		cursor = *page.Cursor
	}
}

func (s *VibeStorage) Delete(ctx context.Context, key string) error {
	// A slash in the key is part of the name, not of the route.
	_, err := s.do(ctx, http.MethodDelete, "/storage/objects/"+strings.ReplaceAll(url.PathEscape(key), "/", "%2F"), "", nil)
	return err
}

func (s *VibeStorage) do(ctx context.Context, method, path, contentType string, body io.Reader) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = apiBase
	}
	request, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return nil, fmt.Errorf("vibe storage: %w", err)
	}
	request.Header.Set("X-Api-Key", s.APIKey)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("vibe storage: %w", err)
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("vibe storage: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("vibe storage: %s %s: %s: %.300s", method, strings.SplitN(path, "?", 2)[0], response.Status, strings.TrimSpace(string(answer)))
	}
	return answer, nil
}
