package client

// Collections: several uploads behind one link.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// CollectionFile is one member of a collection.
type CollectionFile struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type,omitempty"`
	Encrypted   bool   `json:"encrypted"`
}

// Collection is one link standing for several uploads.
type Collection struct {
	URL        string           `json:"url"`
	DeleteURL  string           `json:"delete_url,omitempty"`
	ArchiveURL string           `json:"archive_url"`
	Name       string           `json:"name,omitempty"`
	Files      []CollectionFile `json:"files"`
	TotalSize  int64            `json:"total_size"`
}

// CreateCollection groups uploads that already exist behind one link. The files
// are named by their share URLs, so nothing is re-uploaded.
func (c *Client) CreateCollection(ctx context.Context, name string, files []string) (*Collection, error) {
	if len(files) == 0 {
		return nil, errors.New("a collection needs at least one file")
	}

	body, err := json.Marshal(struct {
		Name  string   `json:"name,omitempty"`
		Files []string `json:"files"`
	}{Name: name, Files: files})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/collection", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	res, err := c.do(req)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusMethodNotAllowed) {
			return nil, errors.New("this server does not support collections")
		}
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	var collection Collection
	if err := json.NewDecoder(res.Body).Decode(&collection); err != nil {
		return nil, err
	}
	if collection.URL == "" {
		return nil, errors.New("server did not return a collection URL")
	}

	return &collection, nil
}
