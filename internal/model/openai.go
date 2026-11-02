package model

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

const DefaultModel = "gpt-6.1-sol"

const (
	DefaultReasoningEffort = "medium"
	DefaultTextVerbosity   = "medium"
	DefaultMaxOutputTokens = int64(8192)
	DefaultMaxRetries      = 2
)

type Config struct {
	Provider        ProviderID
	Model           string
	BaseURL         string
	APIKey          string `json:"-"`
	Timeout         time.Duration
	ReasoningEffort string
	TextVerbosity   string
	MaxOutputTokens int64
	MaxRetries      int
	HTTPClient      *http.Client `json:"-"`
}

// FromEnvironment reads the CLI model and key without storing credentials.
func FromEnvironment() Config {
	provider := ProviderID(envString("AETHER_MODEL_PROVIDER", string(ProviderOpenAI)))
	defaultModel := DefaultModel
	keyVariable := "OPENAI_API_KEY"
	switch provider {
	case ProviderOpenRouter:
		defaultModel = "openai/gpt-6.1-sol"
		keyVariable = "OPENROUTER_API_KEY"
	case ProviderCompatible:
		defaultModel = ""
		keyVariable = "AETHER_COMPATIBLE_API_KEY"
	}
	modelName := strings.TrimSpace(os.Getenv("AETHER_MODEL_NAME"))
	if modelName == "" {
		modelName = defaultModel
	}
	return Config{
		Provider:        provider,
		Model:           modelName,
		BaseURL:         strings.TrimSpace(os.Getenv("AETHER_MODEL_BASE_URL")),
		APIKey:          strings.TrimSpace(os.Getenv(keyVariable)),
		Timeout:         envDurationSeconds("AETHER_MODEL_TIMEOUT_SECONDS", 90),
		ReasoningEffort: envString("AETHER_REASONING_EFFORT", DefaultReasoningEffort),
		TextVerbosity:   envString("AETHER_TEXT_VERBOSITY", DefaultTextVerbosity),
		MaxOutputTokens: envInt64("AETHER_MAX_OUTPUT_TOKENS", DefaultMaxOutputTokens),
		MaxRetries:      envInt("AETHER_MAX_RETRIES", DefaultMaxRetries),
	}
}

func envString(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envInt64(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return -1
	}
	return n
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return n
}

func envDurationSeconds(name string, fallback int64) time.Duration {
	seconds := envInt64(name, fallback)
	if seconds <= 0 {
		return -time.Second
	}
	return time.Duration(seconds) * time.Second
}

type Generator struct {
	Config Config
}

func (g Generator) Readiness() (bool, string) {
	config := normalizeConfig(g.Config)
	if strings.TrimSpace(config.APIKey) == "" {
		return false, "OpenAI API key is missing; add one in Settings or set OPENAI_API_KEY."
	}
	if strings.TrimSpace(config.Model) == "" {
		return false, "OpenAI model name is empty."
	}
	if !oneOf(config.ReasoningEffort, "none", "minimal", "low", "medium", "high", "xhigh", "max") {
		return false, "reasoning effort must be none, minimal, low, medium, high, xhigh, or max."
	}
	if !oneOf(config.TextVerbosity, "low", "medium", "high") {
		return false, "text verbosity must be low, medium, or high."
	}
	if config.MaxOutputTokens < 256 || config.MaxOutputTokens > 65536 {
		return false, "maximum output tokens must be between 256 and 65536."
	}
	if config.MaxRetries < 0 || config.MaxRetries > 5 {
		return false, "maximum retries must be between 0 and 5."
	}
	if config.Timeout < 0 {
		return false, "model timeout must be greater than zero."
	}
	return true, ""
}

func (g Generator) Description() string {
	return "OpenAI Responses API (" + g.Config.Model + ")"
}

func (g Generator) Generate(ctx context.Context, objective string, onDelta func(string)) (string, error) {
	if ready, reason := g.Readiness(); !ready {
		return "", errors.New(reason)
	}
	client := g.client()
	userPrompt := "Use the Dot profile and conversation below to answer the newest user message. Treat the conversation as an ongoing chat. Be clear about uncertainty, ask a concise follow-up when necessary, and never claim to have accessed local files, apps, accounts, or taken external actions.\n\n" + objective
	config := normalizeConfig(g.Config)
	redactor := secretRedactor{secret: strings.TrimSpace(config.APIKey)}

	stream := client.Responses.NewStreaming(ctx, responses.ResponseNewParams{
		Model:           strings.TrimSpace(config.Model),
		Instructions:    openai.String("You are a helpful, careful AI teammate in AETHER Dots. Follow the user's Dot profile and the conversation context. You have no access to the user's files, folders, apps, or accounts, and you cannot take actions outside this chat. Follow an explicitly selected skill's approval boundaries, and never imply its instructions gave you access or authority you do not have."),
		Input:           responses.ResponseNewParamsInputUnion{OfString: openai.String(userPrompt)},
		Store:           param.NewOpt(false),
		MaxOutputTokens: param.NewOpt(config.MaxOutputTokens),
		Reasoning:       shared.ReasoningParam{Effort: shared.ReasoningEffort(config.ReasoningEffort)},
		Text:            responses.ResponseTextConfigParam{Verbosity: responses.ResponseTextConfigVerbosity(config.TextVerbosity)},
	})
	defer func() { _ = stream.Close() }()

	var result strings.Builder
	for stream.Next() {
		event := stream.Current()
		if event.Type == "error" {
			message := strings.TrimSpace(redactProviderText(event.Message, config.APIKey))
			if message == "" {
				message = "OpenAI Responses API returned an error."
			}
			return "", errors.New(message)
		}
		if event.Type != "response.output_text.delta" || event.Delta == "" {
			continue
		}
		result.WriteString(event.Delta)
		if onDelta != nil {
			if safeDelta := redactor.push(event.Delta, false); safeDelta != "" {
				onDelta(safeDelta)
			}
		}
		if result.Len() > 2*1024*1024 {
			return "", errors.New("model response exceeded the 2 MiB limit")
		}
	}
	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("OpenAI response stream: %w", redactProviderError(err, config.APIKey))
	}
	response := redactProviderText(result.String(), config.APIKey)
	if strings.TrimSpace(response) == "" {
		return "", errors.New("OpenAI returned an empty response")
	}
	if onDelta != nil {
		if safeDelta := redactor.push("", true); safeDelta != "" {
			onDelta(safeDelta)
		}
	}
	return response, nil
}

func (g Generator) Test(ctx context.Context) error {
	text, err := g.Generate(ctx, "Newest user message:\nReply with the exact text AETHER_OK.", nil)
	if err != nil {
		return err
	}
	if !strings.Contains(text, "AETHER_OK") {
		return errors.New("OpenAI replied, but did not return the expected test marker")
	}
	return nil
}

func (g Generator) client() openai.Client {
	config := normalizeConfig(g.Config)
	client := config.HTTPClient
	if client == nil {
		client = newProviderHTTPClient(config.Timeout)
	}
	return openai.NewClient(
		option.WithAPIKey(strings.TrimSpace(config.APIKey)),
		option.WithHTTPClient(client),
		option.WithMaxRetries(config.MaxRetries),
	)
}

func normalizeConfig(config Config) Config {
	if strings.TrimSpace(config.ReasoningEffort) == "" {
		config.ReasoningEffort = DefaultReasoningEffort
	}
	if strings.TrimSpace(config.TextVerbosity) == "" {
		config.TextVerbosity = DefaultTextVerbosity
	}
	if config.MaxOutputTokens == 0 {
		config.MaxOutputTokens = DefaultMaxOutputTokens
	}
	if config.MaxRetries == 0 && config.Timeout <= 0 {
		// A zero Config is primarily used by embedded callers and tests. The
		// explicit environment and settings paths can still select zero retries.
		config.MaxRetries = DefaultMaxRetries
	}
	if config.Timeout == 0 {
		config.Timeout = 90 * time.Second
	}
	return config
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
