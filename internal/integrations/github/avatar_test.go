package github

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type githubRoundTripFunc func(*http.Request) (*http.Response, error)

func (f githubRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestFetchAvatarUsesUnauthenticatedBoundedGitHubRasterOnly(t *testing.T) {
	var calls atomic.Int32
	imageBytes := testAvatarPNG(t, 16, 16)
	client := &http.Client{Transport: githubRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.URL.Host != "avatars.githubusercontent.com" || request.Header.Get("Authorization") != "" {
			t.Fatalf("avatar request was not isolated from API credentials: %s auth=%q", request.URL, request.Header.Get("Authorization"))
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader(string(imageBytes)))}, nil
	})}
	data, err := FetchAvatar(context.Background(), "https://avatars.githubusercontent.com/u/1?v=4", client)
	if err != nil || len(data) == 0 || calls.Load() != 1 {
		t.Fatalf("avatar bytes=%d calls=%d err=%v", len(data), calls.Load(), err)
	}
	clear(data)
	if _, err := FetchAvatar(context.Background(), "https://example.com/avatar.png", client); err == nil || calls.Load() != 1 {
		t.Fatalf("arbitrary avatar URL reached network: calls=%d err=%v", calls.Load(), err)
	}
	large := strings.Repeat("x", 256*1024+1)
	client.Transport = githubRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader(large))}, nil
	})
	if _, err := FetchAvatar(context.Background(), "https://avatars.githubusercontent.com/u/1", client); err == nil || !strings.Contains(err.Error(), "256 KiB") {
		t.Fatalf("oversized avatar was accepted: %v", err)
	}
}

func testAvatarPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buffer strings.Builder
	imageData := image.NewRGBA(image.Rect(0, 0, width, height))
	imageData.Set(0, 0, color.RGBA{R: 20, G: 80, B: 120, A: 255})
	if err := png.Encode(&stringWriter{Builder: &buffer}, imageData); err != nil {
		t.Fatal(err)
	}
	return []byte(buffer.String())
}

type stringWriter struct{ *strings.Builder }

func (w *stringWriter) Write(data []byte) (int, error) { return w.Builder.Write(data) }
