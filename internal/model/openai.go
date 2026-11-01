package model

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

const DefaultModel = "gpt-6.1-sol"

type Config struct {
	Model      string
	APIKey     string `json:"-"`
	Timeout    time.Duration
	HTTPClient *http.Client `json:"-"`
}

// FromEnvironment reads the CLI model and key without storing credentials.
func FromEnvironment() Config {
	modelName := strings.TrimSpace(os.Getenv("AETHER_MODEL_NAME"))
	if modelName == "" {
		modelName = DefaultModel
	}
	return Config{
		Model:   modelName,
		APIKey:  strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		Timeout: 90 * time.Second,
	}
}

type Generator struct {
	Config Config
}

func (g Generator) Readiness() (bool, string) {
	if strings.TrimSpace(g.Config.APIKey) == "" {
		return false, "OpenAI API key is missing; add one in Settings or set OPENAI_API_KEY."
	}
	if strings.TrimSpace(g.Config.Model) == "" {
		return false, "OpenAI model name is empty."
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

	stream := client.Responses.NewStreaming(ctx, responses.ResponseNewParams{
		Model:        strings.TrimSpace(g.Config.Model),
		Instructions: openai.String("You are a helpful, careful AI teammate in AETHER Dots. Follow the user's Dot profile and the conversation context. You have no access to the user's files, folders, apps, or accounts, and you cannot take actions outside this chat."),
		Input:        responses.ResponseNewParamsInputUnion{OfString: openai.String(userPrompt)},
		Store:        param.NewOpt(false),
	})
	defer func() { _ = stream.Close() }()

	var result strings.Builder
	for stream.Next() {
		event := stream.Current()
		if event.Type == "error" {
			message := strings.TrimSpace(event.Message)
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
			onDelta(event.Delta)
		}
		if result.Len() > 2*1024*1024 {
			return "", errors.New("model response exceeded the 2 MiB limit")
		}
	}
	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("OpenAI response stream: %w", err)
	}
	if strings.TrimSpace(result.String()) == "" {
		return "", errors.New("OpenAI returned an empty response")
	}
	return result.String(), nil
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
	timeout := g.Config.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	client := g.Config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return openai.NewClient(
		option.WithAPIKey(g.Config.APIKey),
		option.WithHTTPClient(client),
	)
}
