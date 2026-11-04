package model

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

type ProviderID string

const (
	ProviderOpenAI       ProviderID = "openai"
	ProviderAnthropic    ProviderID = "anthropic"
	ProviderXAI          ProviderID = "xai"
	ProviderOpenRouter   ProviderID = "openrouter"
	ProviderCompatible   ProviderID = "openai-compatible"
	DefaultOpenRouterURL            = "https://openrouter.ai/api/v1"
	DefaultXAIURL                   = "https://api.x.ai/v1"
)

// Provider is the provider-neutral contract used by chat and scheduled runs.
// It deliberately carries no conversation or provider identity into the store.
type Provider interface {
	Generate(context.Context, string, func(string)) (string, error)
	Readiness() (bool, string)
	Description() string
	Test(context.Context) error
}

// managedProvider keeps request-scoped tools and transports alive for exactly
// one run, then releases them when the engine calls Close.
type managedProvider struct {
	base    Provider
	tools   []Tool
	cleanup []func()
	once    *sync.Once
}

func (p managedProvider) Generate(ctx context.Context, prompt string, onDelta func(string)) (string, error) {
	return WithTools(p.base, p.tools...).Generate(ctx, prompt, onDelta)
}

func (p managedProvider) Readiness() (bool, string) { return p.base.Readiness() }
func (p managedProvider) Description() string       { return p.base.Description() }
func (p managedProvider) Test(ctx context.Context) error {
	return p.base.Test(ctx)
}

func (p managedProvider) Close() error {
	if p.once == nil {
		return nil
	}
	p.once.Do(func() {
		for i := len(p.cleanup) - 1; i >= 0; i-- {
			p.cleanup[i]()
		}
	})
	return nil
}

// WithCleanup attaches run-scoped cleanup to a provider. Multiple decorations
// preserve one lifecycle and close resources once after the run.
func WithCleanup(provider Provider, cleanup func()) Provider {
	if cleanup == nil {
		return provider
	}
	if current, ok := provider.(managedProvider); ok {
		current.cleanup = append(current.cleanup, cleanup)
		return current
	}
	return managedProvider{base: provider, cleanup: []func(){cleanup}, once: &sync.Once{}}
}

// NewProvider builds a concrete adapter while keeping provider selection out
// of persisted conversation and Dot data.
func NewProvider(config Config) Provider {
	if config.Provider == "" {
		config.Provider = ProviderOpenAI
	}
	switch config.Provider {
	case ProviderOpenAI:
		return Generator{Config: config}
	case ProviderXAI:
		if strings.TrimSpace(config.Model) == "" {
			config.Model = "grok-4.7"
		}
		config.BaseURL = DefaultXAIURL
		return Generator{Config: config}
	case ProviderAnthropic:
		if strings.TrimSpace(config.Model) == "" {
			config.Model = "claude-sonnet-5-5"
		}
		return AnthropicGenerator{Config: config}
	case ProviderOpenRouter, ProviderCompatible:
		return CompatibleGenerator{Config: config}
	default:
		return invalidProvider{provider: config.Provider}
	}
}

// WithPromptCacheKey configures a provider's in-memory prompt affinity key
// where its API supports one. It does not enable server-side conversation storage.
func WithPromptCacheKey(provider Provider, key string) Provider {
	key = strings.TrimSpace(key)
	switch current := provider.(type) {
	case managedProvider:
		current.base = WithPromptCacheKey(current.base, key)
		return current
	case Generator:
		current.Config.PromptCacheKey = key
		return current
	case CompatibleGenerator:
		current.Config.PromptCacheKey = key
		return current
	case AnthropicGenerator:
		current.Config.PromptCacheKey = key
		return current
	default:
		return provider
	}
}

type invalidProvider struct{ provider ProviderID }

func (p invalidProvider) message() string {
	return fmt.Sprintf("unsupported model provider %q", p.provider)
}

func (p invalidProvider) Generate(context.Context, string, func(string)) (string, error) {
	return "", errors.New(p.message())
}

func (p invalidProvider) Readiness() (bool, string) { return false, p.message() }
func (p invalidProvider) Description() string       { return string(p.provider) }
func (p invalidProvider) Test(context.Context) error {
	return errors.New(p.message())
}

// ValidateBaseURL ensures a custom endpoint cannot smuggle credentials in its
// URL or send a token over cleartext to a non-local host. Loopback HTTP is
// allowed for local OpenAI-compatible servers.
func ValidateBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("custom provider endpoint is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Hostname() == "" {
		return "", errors.New("custom provider endpoint must be an absolute URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("custom provider endpoint cannot contain credentials, a query, or a fragment")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("custom provider endpoint must use HTTPS (HTTP is allowed only for localhost)")
	}
	if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
		return "", errors.New("custom provider endpoint must use HTTPS unless it is a loopback address")
	}
	if port := u.Port(); port != "" {
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return "", errors.New("custom provider endpoint has an invalid port")
		}
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func redactProviderText(text, apiKey string) string {
	secret := strings.TrimSpace(apiKey)
	if secret == "" {
		return text
	}
	return strings.ReplaceAll(text, secret, "[REDACTED]")
}

func redactProviderError(err error, apiKey string) error {
	if err == nil {
		return nil
	}
	message := redactProviderText(err.Error(), apiKey)
	if message == err.Error() {
		return err
	}
	var cause error
	if errors.Is(err, context.Canceled) {
		cause = context.Canceled
	} else if errors.Is(err, context.DeadlineExceeded) {
		cause = context.DeadlineExceeded
	}
	return redactedProviderError{message: message, cause: cause}
}

type redactedProviderError struct {
	message string
	cause   error
}

func (e redactedProviderError) Error() string { return e.message }
func (e redactedProviderError) Unwrap() error { return e.cause }

// secretRedactor keeps enough trailing bytes to catch a credential split
// across streamed text chunks before anything reaches the UI.
type secretRedactor struct {
	secret  string
	pending string
}

func (r *secretRedactor) push(chunk string, final bool) string {
	if r.secret == "" {
		return chunk
	}
	r.pending += chunk
	safeLength := len(r.pending)
	if !final {
		safeLength -= len(r.secret) - 1
		if safeLength < 0 {
			safeLength = 0
		}
		maxOverlap := min(len(r.secret)-1, safeLength)
		for overlap := maxOverlap; overlap > 0; overlap-- {
			if strings.HasSuffix(r.pending[:safeLength], r.secret[:overlap]) {
				safeLength -= overlap
				break
			}
		}
		if safeLength < len(r.pending) {
			for safeLength > 0 && !utf8.RuneStart(r.pending[safeLength]) {
				safeLength--
			}
		}
	}
	output := redactProviderText(r.pending[:safeLength], r.secret)
	r.pending = r.pending[safeLength:]
	if final {
		output += redactProviderText(r.pending, r.secret)
		r.pending = ""
	}
	return output
}
