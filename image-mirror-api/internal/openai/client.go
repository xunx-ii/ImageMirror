package openai

import (
	"bufio"
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
	endpoints     EndpointProvider
	reportAttempt func(ctx context.Context, endpointID string)
	reportSuccess func(ctx context.Context, endpointID string)
	reportFailure func(ctx context.Context, endpointID string, message string)
	http          *http.Client
}

type EndpointProvider func(ctx context.Context) ([]Endpoint, error)

type Endpoint struct {
	ID                string
	Name              string
	APIKey            string
	BaseURL           string
	SupportsStreaming bool
}

type EndpointReporter struct {
	Attempt func(ctx context.Context, endpointID string)
	Success func(ctx context.Context, endpointID string)
	Failure func(ctx context.Context, endpointID string, message string)
}

type ImageRequest struct {
	Model         string `json:"model"`
	Prompt        string `json:"prompt"`
	Size          string `json:"size,omitempty"`
	Quality       string `json:"quality,omitempty"`
	N             int    `json:"n,omitempty"`
	Stream        bool   `json:"stream,omitempty"`
	PartialImages int    `json:"partial_images,omitempty"`
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

type imageStreamEvent struct {
	Type    string `json:"type"`
	B64JSON string `json:"b64_json"`
	URL     string `json:"url"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type APIError struct {
	StatusCode int
	Message    string
	Retryable  bool
}

func (e APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("openai error: %s", e.Message)
	}
	return fmt.Sprintf("openai error status %d", e.StatusCode)
}

func NewClient(timeout time.Duration, endpoints EndpointProvider, reporter EndpointReporter) *Client {
	return &Client{
		endpoints:     endpoints,
		reportAttempt: reporter.Attempt,
		reportSuccess: reporter.Success,
		reportFailure: reporter.Failure,
		http:          &http.Client{Timeout: timeout},
	}
}

func (c *Client) GenerateImage(ctx context.Context, req ImageRequest, references []ReferenceImage) ([]byte, error) {
	if req.Model == "" {
		req.Model = "gpt-image-2"
	}
	if req.N == 0 {
		req.N = 1
	}
	endpoints, err := c.endpoints(ctx)
	if err != nil {
		return nil, err
	}
	if len(endpoints) == 0 {
		return nil, errors.New("openai api key is not configured")
	}

	var lastErr error
	for _, endpoint := range endpoints {
		endpoint.APIKey = strings.TrimSpace(endpoint.APIKey)
		endpoint.BaseURL = normalizeBaseURL(endpoint.BaseURL)
		if endpoint.APIKey == "" {
			continue
		}
		if endpoint.BaseURL == "" {
			endpoint.BaseURL = "https://api.openai.com"
		}
		if c.reportAttempt != nil {
			c.reportAttempt(ctx, endpoint.ID)
		}

		stream := endpoint.SupportsStreaming
		request := req
		request.Stream = stream
		request.PartialImages = 0

		var data []byte
		if len(references) > 0 {
			data, err = c.editImage(ctx, endpoint.BaseURL, endpoint.APIKey, request, references)
		} else {
			data, err = c.generateImage(ctx, endpoint.BaseURL, endpoint.APIKey, request)
		}
		if err == nil {
			if c.reportSuccess != nil {
				c.reportSuccess(ctx, endpoint.ID)
			}
			return data, nil
		}
		if !isEndpointRetryable(err) {
			return nil, err
		}
		lastErr = err
		if c.reportFailure != nil {
			c.reportFailure(ctx, endpoint.ID, err.Error())
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("all openai endpoints failed: %w", lastErr)
	}
	return nil, errors.New("openai api key is not configured")
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
	if req.Stream {
		return c.doImageStreamRequest(ctx, httpReq)
	}
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
	if req.Stream {
		fields["stream"] = "true"
		fields["partial_images"] = fmt.Sprintf("%d", req.PartialImages)
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
	if req.Stream {
		return c.doImageStreamRequest(ctx, httpReq)
	}
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, retryable := openAIErrorMessage(payload, resp.Header.Get("Content-Type"))
		return nil, APIError{StatusCode: resp.StatusCode, Message: message, Retryable: retryable}
	}
	if !looksLikeJSON(payload) {
		return nil, APIError{StatusCode: http.StatusBadGateway, Message: nonJSONResponseMessage(payload, resp.Header.Get("Content-Type")), Retryable: true}
	}
	return c.decodeImagePayload(ctx, payload)
}

func (c *Client) doImageStreamRequest(ctx context.Context, httpReq *http.Request) ([]byte, error) {
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, readErr
		}
		message, retryable := openAIErrorMessage(payload, resp.Header.Get("Content-Type"))
		return nil, APIError{StatusCode: resp.StatusCode, Message: message, Retryable: retryable}
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		payload, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		if !looksLikeJSON(payload) {
			return nil, APIError{StatusCode: http.StatusBadGateway, Message: nonJSONResponseMessage(payload, resp.Header.Get("Content-Type")), Retryable: true}
		}
		return c.decodeImagePayload(ctx, payload)
	}
	return c.decodeImageStream(ctx, resp.Body)
}

func (c *Client) decodeImagePayload(ctx context.Context, payload []byte) ([]byte, error) {
	var decoded imageResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, APIError{StatusCode: http.StatusBadGateway, Message: "image API returned invalid JSON: " + responseSnippet(payload), Retryable: true}
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

func (c *Client) decodeImageStream(ctx context.Context, stream io.Reader) ([]byte, error) {
	reader := bufio.NewReader(stream)
	var dataLines []string
	var lastImage []byte
	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if errors.Is(err, io.EOF) {
				if len(lastImage) > 0 {
					return lastImage, nil
				}
				return nil, errors.New("openai image stream ended without image data")
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(dataLines) > 0 {
				image, done, err := c.decodeImageStreamEvent(ctx, strings.Join(dataLines, "\n"))
				dataLines = dataLines[:0]
				if err != nil {
					return nil, err
				}
				if len(image) > 0 {
					lastImage = image
				}
				if done && len(lastImage) > 0 {
					return lastImage, nil
				}
			}
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				if len(lastImage) > 0 {
					return lastImage, nil
				}
				return nil, errors.New("openai image stream finished without image data")
			}
			dataLines = append(dataLines, data)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(lastImage) > 0 {
					return lastImage, nil
				}
				return nil, errors.New("openai image stream ended without image data")
			}
			return nil, err
		}
	}
}

func (c *Client) decodeImageStreamEvent(ctx context.Context, payload string) ([]byte, bool, error) {
	if strings.TrimSpace(payload) == "" {
		return nil, false, nil
	}
	var event imageStreamEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return nil, false, APIError{StatusCode: http.StatusBadGateway, Message: "image stream returned invalid JSON: " + responseSnippet([]byte(payload)), Retryable: true}
	}
	if event.Error != nil {
		message := strings.TrimSpace(event.Error.Message)
		if message == "" {
			message = "image stream returned an error"
		}
		return nil, false, errors.New(message)
	}
	done := event.Type == "image_generation.completed" || event.Type == "image_edit.completed"
	if event.B64JSON != "" {
		data, err := base64.StdEncoding.DecodeString(event.B64JSON)
		if err != nil {
			return nil, done, err
		}
		return data, done, nil
	}
	if event.URL != "" {
		data, err := c.download(ctx, event.URL)
		return data, done, err
	}
	return nil, done, nil
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

func isEndpointRetryable(err error) bool {
	var apiErr APIError
	if !errors.As(err, &apiErr) {
		return true
	}
	if apiErr.Retryable {
		return true
	}
	switch {
	case apiErr.StatusCode == http.StatusUnauthorized, apiErr.StatusCode == http.StatusForbidden:
		return true
	case apiErr.StatusCode == http.StatusTooManyRequests:
		return true
	case apiErr.StatusCode == http.StatusRequestTimeout:
		return true
	case apiErr.StatusCode >= 500:
		return true
	default:
		return false
	}
}

func openAIErrorMessage(payload []byte, contentType string) (string, bool) {
	if contentTypeIsJSON(contentType) || looksLikeJSON(payload) {
		var decoded imageResponse
		if err := json.Unmarshal(payload, &decoded); err == nil {
			if decoded.Error != nil && strings.TrimSpace(decoded.Error.Message) != "" {
				return strings.TrimSpace(decoded.Error.Message), false
			}
			snippet := responseSnippet(payload)
			if snippet != "" {
				return "image API returned an error response: " + snippet, false
			}
			return "image API returned an empty error response", false
		}
		return "image API returned invalid JSON: " + responseSnippet(payload), true
	}
	return nonJSONResponseMessage(payload, contentType), true
}

func contentTypeIsJSON(contentType string) bool {
	contentType = strings.ToLower(contentType)
	return strings.Contains(contentType, "json")
}

func looksLikeJSON(payload []byte) bool {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] == '{' || trimmed[0] == '['
}

func nonJSONResponseMessage(payload []byte, contentType string) string {
	snippet := responseSnippet(payload)
	if snippet == "" {
		return "image API returned an empty response"
	}
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return "image API returned a non-JSON response: " + snippet
	}
	return fmt.Sprintf("image API returned a non-JSON response (%s): %s", contentType, snippet)
}

func responseSnippet(payload []byte) string {
	snippet := strings.Join(strings.Fields(string(bytes.TrimSpace(payload))), " ")
	if snippet == "" {
		return ""
	}
	runes := []rune(snippet)
	if len(runes) > 240 {
		return string(runes[:240]) + "..."
	}
	return snippet
}
