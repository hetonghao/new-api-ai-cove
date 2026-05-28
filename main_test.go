package main

import (
	"net/http"
	"testing"
)

func TestBuildSessionOptionsDefaults(t *testing.T) {
	t.Setenv("SESSION_COOKIE_DOMAIN", "")
	t.Setenv("SESSION_COOKIE_SECURE", "")

	options := buildSessionOptions()

	if options.Path != "/" {
		t.Fatalf("Path = %q, want /", options.Path)
	}
	if options.Domain != "" {
		t.Fatalf("Domain = %q, want empty host-only cookie", options.Domain)
	}
	if options.MaxAge != 2592000 {
		t.Fatalf("MaxAge = %d, want 2592000", options.MaxAge)
	}
	if !options.HttpOnly {
		t.Fatal("HttpOnly = false, want true")
	}
	if options.Secure {
		t.Fatal("Secure = true, want false by default")
	}
	if options.SameSite != http.SameSiteStrictMode {
		t.Fatalf("SameSite = %v, want Strict", options.SameSite)
	}
}

func TestBuildSessionOptionsAllowsSharedProductionCookie(t *testing.T) {
	t.Setenv("SESSION_COOKIE_DOMAIN", ".ai-cove.com")
	t.Setenv("SESSION_COOKIE_SECURE", "true")

	options := buildSessionOptions()

	if options.Domain != ".ai-cove.com" {
		t.Fatalf("Domain = %q, want .ai-cove.com", options.Domain)
	}
	if !options.Secure {
		t.Fatal("Secure = false, want true")
	}
}
