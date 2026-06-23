package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	credentials CredentialProvider
	http        *http.Client
}

type CredentialProvider func(ctx context.Context) (apiKey string, baseURL string, err error)

type ImageRequest struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	Size    string `json:"size,omitempty"`
	Quality string `json:"quality,omitempty"`
	N       int    `json:"n,omitempty"`
}

type ReferenceImage struct {
	Filename string
	Data     []byte
}

type imageResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func NewClient(timeout time.Duration, credentials CredentialProvider) *Client {
	return &Client{
		credentials: credentials,
		http:        &http.Client{Timeout: timeout},
	}
}

func (c *Client) GenerateImage(ctx context.Context, req ImageRequest, references []ReferenceImage) ([]byte, error) {
	if req.Model == "" {
		req.Model = "gpt-image-2"
	}
	if req.N == 0 {
		req.N = 1
	}
	apiKey, baseURL, err := c.credentials(ctx)
	if err != nil {
		return nil, err
	}
	apiKey = strings.TrimSpace(apiKey)
	baseURL = normalizeBaseURL(baseURL)
	if apiKey == "" {
		return nil, errors.New("openai api key is not configured")
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	if len(references) > 0 {
		return c.editImage(ctx, baseURL, apiKey, req, references)
	}
	return c.generateImage(ctx, baseURL, apiKey, req)
}

func (c *Client) generateImage(ctx context.Context, baseURL string, apiKey string, req ImageRequest) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return c.doImageRequest(ctx, httpReq)
}

func normalizeBaseURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	return strings.TrimSuffix(value, "/v1")
}

func (c *Client) editImage(ctx context.Context, baseURL string, apiKey string, req ImageRequest, references []ReferenceImage) ([]byte, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fields := map[string]string{
		"model":  req.Model,
		"prompt": req.Prompt,
		"n":      fmt.Sprintf("%d", req.N),
	}
	if req.Size != "" {
		fields["size"] = req.Size
	}
	if req.Quality != "" {
		fields["quality"] = req.Quality
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	for i, reference := range references {
		filename := strings.TrimSpace(reference.Filename)
		if filename == "" {
			filename = fmt.Sprintf("reference-%d.png", i+1)
		}
		part, err := writer.CreateFormFile("image[]", filename)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(reference.Data); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/images/edits", body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	return c.doImageRequest(ctx, httpReq)
}

func (c *Client) doImageRequest(ctx context.Context, httpReq *http.Request) ([]byte, error) {
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var decoded imageResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decoded.Error != nil && decoded.Error.Message != "" {
			return nil, fmt.Errorf("openai error: %s", decoded.Error.Message)
		}
		return nil, fmt.Errorf("openai error status %d", resp.StatusCode)
	}
	if len(decoded.Data) == 0 {
		return nil, errors.New("openai returned no image data")
	}
	if decoded.Data[0].B64JSON != "" {
		return base64.StdEncoding.DecodeString(decoded.Data[0].B64JSON)
	}
	if decoded.Data[0].URL != "" {
		return c.download(ctx, decoded.Data[0].URL)
	}
	return nil, errors.New("openai image response missing b64_json")
}

func (c *Client) download(ctx context.Context, imageURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download generated image status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
