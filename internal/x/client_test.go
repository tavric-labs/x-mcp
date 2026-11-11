package x

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProfileSendsTheBearerTokenAndDecodesTheUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/2/users/me") || r.Header.Get("Authorization") != "Bearer member-token" {
			t.Errorf("unexpected request %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":{"id":"42","name":"Ada Lovelace","username":"ada"}}`))
	}))
	defer server.Close()

	profile, err := New(server.URL).Profile(context.Background(), "member-token")
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "42" || profile.Name != "Ada Lovelace" || profile.Username != "ada" {
		t.Fatalf("profile = %+v", profile)
	}
}

func TestAnExpiredOrRevokedTokenIsReportedAsUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"EXPIRED_TOKEN secret-detail"}`))
	}))
	defer server.Close()

	_, err := New(server.URL).Profile(context.Background(), "stale")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if err != nil && strings.Contains(err.Error(), "secret-detail") {
		t.Fatalf("the upstream body leaked into the error: %v", err)
	}
}

func TestCreatePostPublishesAndReturnsThePostID(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/2/tweets" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer member-token" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("headers Authorization=%q Content-Type=%q", r.Header.Get("Authorization"), r.Header.Get("Content-Type"))
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"data":{"id":"191","text":"Hello X"}}`))
	}))
	defer server.Close()

	id, err := New(server.URL).CreatePost(context.Background(), "member-token", "Hello X")
	if err != nil {
		t.Fatal(err)
	}
	if id != "191" {
		t.Fatalf("id = %q", id)
	}
	if body["text"] != "Hello X" {
		t.Fatalf("post body = %v", body)
	}
}

func TestPaymentRequiredIsReported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
	}))
	defer server.Close()
	if _, err := New(server.URL).CreatePost(context.Background(), "t", "hi"); !errors.Is(err, ErrPaymentRequired) {
		t.Fatalf("err = %v, want ErrPaymentRequired", err)
	}
}
