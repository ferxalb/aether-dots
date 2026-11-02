package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	openRouterModelPageSize = 1000
	openRouterModelMaxPages = 20
	openRouterModelMaxBody  = 8 << 20
)

// OpenRouterModel is the subset of live catalog metadata AETHER needs to
// present text-chat models and configure optional reasoning and native tools.
type OpenRouterModel struct {
	ID                        string
	Name                      string
	ContextLength             int64
	SupportsTools             bool
	SupportsReasoning         bool
	ReasoningMetadataKnown    bool
	SupportedReasoningEfforts []string
}

type openRouterModelData struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	ContextLength       int64    `json:"context_length"`
	Endpoints           []string `json:"endpoints"`
	SupportedParameters []string `json:"supported_parameters"`
	Reasoning           struct {
		SupportedEfforts []string `json:"supported_efforts"`
	} `json:"reasoning"`
	Architecture struct {
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
}

type openRouterModelsResponse struct {
	Data []openRouterModelData `json:"data"`
}

// FetchOpenRouterModels loads OpenRouter's text-input, text-output models that
// support the Chat Completions endpoint used by AETHER. The API key is sent
// only in the Authorization header and is never included in returned errors.
func FetchOpenRouterModels(ctx context.Context, apiKey string) ([]OpenRouterModel, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("OpenRouter API key is required to load its model catalog")
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	models := make([]OpenRouterModel, 0, 512)
	seen := make(map[string]struct{})
	for page := 0; page < openRouterModelMaxPages; page++ {
		items, err := fetchOpenRouterModelPage(ctx, client, apiKey, page*openRouterModelPageSize)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.ID == "" || !containsString(item.Architecture.InputModalities, "text") ||
				!containsString(item.Architecture.OutputModalities, "text") ||
				!containsString(item.Endpoints, "/api/v1/chat/completions") {
				continue
			}
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			name := strings.TrimSpace(item.Name)
			if name == "" {
				name = item.ID
			}
			models = append(models, OpenRouterModel{
				ID: item.ID, Name: name, ContextLength: item.ContextLength,
				SupportsTools:             containsString(item.SupportedParameters, "tools"),
				SupportsReasoning:         containsString(item.SupportedParameters, "reasoning") || containsString(item.SupportedParameters, "reasoning_effort") || len(item.Reasoning.SupportedEfforts) > 0,
				ReasoningMetadataKnown:    item.SupportedParameters != nil || item.Reasoning.SupportedEfforts != nil,
				SupportedReasoningEfforts: append([]string(nil), item.Reasoning.SupportedEfforts...),
			})
		}
		if len(items) < openRouterModelPageSize {
			return models, nil
		}
	}
	return nil, errors.New("OpenRouter model catalog exceeded the safe pagination limit")
}

func fetchOpenRouterModelPage(ctx context.Context, client *http.Client, apiKey string, offset int) ([]openRouterModelData, error) {
	endpoint, err := url.Parse(DefaultOpenRouterURL + "/models")
	if err != nil {
		return nil, errors.New("OpenRouter model catalog URL is invalid")
	}
	query := endpoint.Query()
	query.Set("input_modalities", "text")
	query.Set("output_modalities", "text")
	query.Set("sort", "most-popular")
	query.Set("limit", strconv.Itoa(openRouterModelPageSize))
	query.Set("offset", strconv.Itoa(offset))
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, errors.New("could not prepare OpenRouter model catalog request")
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("OpenRouter model catalog request failed: %w", redactProviderError(err, apiKey))
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenRouter model catalog returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, openRouterModelMaxBody+1))
	if err != nil {
		return nil, fmt.Errorf("read OpenRouter model catalog: %w", redactProviderError(err, apiKey))
	}
	if len(body) > openRouterModelMaxBody {
		return nil, errors.New("OpenRouter model catalog response exceeded the 8 MiB safety limit")
	}
	var decoded openRouterModelsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, errors.New("OpenRouter returned an invalid model catalog response")
	}
	return decoded.Data, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
