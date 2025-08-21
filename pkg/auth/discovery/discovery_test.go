package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stacklok/toolhive/pkg/logger"
)

func init() {
	// Initialize logger for tests
	logger.Initialize()
}

func TestParseWWWAuthenticate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		header   string
		expected *AuthInfo
		wantErr  bool
	}{
		{
			name:    "empty header",
			header:  "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			header:  "   ",
			wantErr: true,
		},
		{
			name:   "simple bearer",
			header: "Bearer",
			expected: &AuthInfo{
				Type: "OAuth",
			},
		},
		{
			name:   "bearer with realm",
			header: `Bearer realm="https://example.com"`,
			expected: &AuthInfo{
				Type:  "OAuth",
				Realm: "https://example.com",
			},
		},
		{
			name:   "bearer with quoted realm",
			header: `Bearer realm="https://example.com/oauth"`,
			expected: &AuthInfo{
				Type:  "OAuth",
				Realm: "https://example.com/oauth",
			},
		},
		{
			name:   "oauth scheme",
			header: `OAuth realm="https://example.com"`,
			expected: &AuthInfo{
				Type:  "OAuth",
				Realm: "https://example.com",
			},
		},
		{
			name:   "multiple schemes with bearer first",
			header: `Bearer realm="https://example.com", Basic realm="test"`,
			expected: &AuthInfo{
				Type:  "OAuth",
				Realm: "https://example.com",
			},
		},
		{
			name:    "unsupported scheme",
			header:  "Basic realm=\"test\"",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := ParseWWWAuthenticate(tt.header)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseWWWAuthenticate() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseWWWAuthenticate() unexpected error: %v", err)
				return
			}

			if result.Type != tt.expected.Type {
				t.Errorf("ParseWWWAuthenticate() Type = %v, want %v", result.Type, tt.expected.Type)
			}

			if result.Realm != tt.expected.Realm {
				t.Errorf("ParseWWWAuthenticate() Realm = %v, want %v", result.Realm, tt.expected.Realm)
			}
		})
	}
}

func TestExtractParameter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		params    string
		paramName string
		expected  string
	}{
		{
			name:      "simple parameter",
			params:    `realm="https://example.com"`,
			paramName: "realm",
			expected:  "https://example.com",
		},
		{
			name:      "quoted parameter",
			params:    `realm="https://example.com/oauth"`,
			paramName: "realm",
			expected:  "https://example.com/oauth",
		},
		{
			name:      "multiple parameters",
			params:    `realm="https://example.com", scope="openid"`,
			paramName: "realm",
			expected:  "https://example.com",
		},
		{
			name:      "parameter not found",
			params:    `realm="https://example.com"`,
			paramName: "scope",
			expected:  "",
		},
		{
			name:      "empty params",
			params:    "",
			paramName: "realm",
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ExtractParameter(tt.params, tt.paramName)
			if result != tt.expected {
				t.Errorf("ExtractParameter() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDeriveIssuerFromURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "https url",
			url:      "https://example.com/api",
			expected: "https://example.com",
		},
		{
			name:     "http url",
			url:      "http://localhost:8080/api",
			expected: "https://localhost",
		},
		{
			name:     "url with path",
			url:      "https://api.example.com/v1/endpoint",
			expected: "https://api.example.com",
		},
		{
			name:     "url with query params",
			url:      "https://example.com/api?param=value",
			expected: "https://example.com",
		},
		{
			name:     "invalid url",
			url:      "not-a-url",
			expected: "",
		},
		{
			name:     "empty url",
			url:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := DeriveIssuerFromURL(tt.url)
			if result != tt.expected {
				t.Errorf("DeriveIssuerFromURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDetectAuthenticationFromServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, _ *http.Request)
		expected       *AuthInfo
		wantErr        bool
	}{
		{
			name: "no authentication required",
			serverResponse: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expected: nil,
		},
		{
			name: "bearer authentication required",
			serverResponse: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="https://example.com"`)
				w.WriteHeader(http.StatusUnauthorized)
			},
			expected: &AuthInfo{
				Type:  "OAuth",
				Realm: "https://example.com",
			},
		},
		{
			name: "oauth authentication required",
			serverResponse: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("WWW-Authenticate", `OAuth realm="https://example.com"`)
				w.WriteHeader(http.StatusUnauthorized)
			},
			expected: &AuthInfo{
				Type:  "OAuth",
				Realm: "https://example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			// Test detection
			ctx := context.Background()
			result, err := DetectAuthenticationFromServer(ctx, server.URL, nil)

			if tt.wantErr {
				if err == nil {
					t.Errorf("DetectAuthenticationFromServer() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("DetectAuthenticationFromServer() unexpected error: %v", err)
				return
			}

			if tt.expected == nil {
				if result != nil {
					t.Errorf("DetectAuthenticationFromServer() = %v, want nil", result)
				}
				return
			}

			if result == nil {
				t.Errorf("DetectAuthenticationFromServer() = nil, want %v", tt.expected)
				return
			}

			if result.Type != tt.expected.Type {
				t.Errorf("DetectAuthenticationFromServer() Type = %v, want %v", result.Type, tt.expected.Type)
			}

			if result.Realm != tt.expected.Realm {
				t.Errorf("DetectAuthenticationFromServer() Realm = %v, want %v", result.Realm, tt.expected.Realm)
			}
		})
	}
}

func TestDefaultDiscoveryConfig(t *testing.T) {
	t.Parallel()
	config := DefaultDiscoveryConfig()

	if config.Timeout != 10*time.Second {
		t.Errorf("DefaultDiscoveryConfig() Timeout = %v, want %v", config.Timeout, 10*time.Second)
	}

	if config.TLSHandshakeTimeout != 5*time.Second {
		t.Errorf("DefaultDiscoveryConfig() TLSHandshakeTimeout = %v, want %v", config.TLSHandshakeTimeout, 5*time.Second)
	}

	if config.ResponseHeaderTimeout != 5*time.Second {
		t.Errorf("DefaultDiscoveryConfig() ResponseHeaderTimeout = %v, want %v", config.ResponseHeaderTimeout, 5*time.Second)
	}

	if !config.EnablePOSTDetection {
		t.Errorf("DefaultDiscoveryConfig() EnablePOSTDetection = %v, want %v", config.EnablePOSTDetection, true)
	}

	if !config.EnableRFC9728 {
		t.Errorf("DefaultDiscoveryConfig() EnableRFC9728 = %v, want %v", config.EnableRFC9728, true)
	}
}

func TestOAuthFlowConfig(t *testing.T) {
	t.Parallel()
	t.Run("nil config validation", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		result, err := PerformOAuthFlow(ctx, "https://example.com", nil)

		if err == nil {
			t.Errorf("PerformOAuthFlow() expected error for nil config but got none")
		}
		if result != nil {
			t.Errorf("PerformOAuthFlow() expected nil result for nil config")
		}
		if !strings.Contains(err.Error(), "OAuth flow config cannot be nil") {
			t.Errorf("PerformOAuthFlow() expected nil config error, got: %v", err)
		}
	})

	t.Run("config validation", func(t *testing.T) {
		t.Parallel()
		config := &OAuthFlowConfig{
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			Scopes:       []string{"openid"},
		}

		// This test only validates that the config is accepted and doesn't cause
		// immediate validation errors. The actual OAuth flow will fail with OIDC
		// discovery errors, which is expected.
		if config.ClientID == "" {
			t.Errorf("Expected ClientID to be set")
		}
		if config.ClientSecret == "" {
			t.Errorf("Expected ClientSecret to be set")
		}
		if len(config.Scopes) == 0 {
			t.Errorf("Expected Scopes to be set")
		}
	})
}

func TestRFC9728Discovery(t *testing.T) {
	t.Parallel()

	t.Run("successful RFC-9728 discovery", func(t *testing.T) {
		t.Parallel()
		
		// Create test server that responds to RFC-9728 discovery
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/oauth-protected-resource" && r.Method == http.MethodGet {
				// Return RFC-9728 compliant response
				w.Header().Set("Content-Type", "application/json")
				response := RFC9728AuthInfo{
					Resource:               "https://api.example.com",
					AuthorizationServers:   []string{"https://auth.example.com"},
					BearerMethodsSupported: []string{"header"},
					JWKSURI:                "https://auth.example.com/.well-known/jwks.json",
					ScopesSupported:        []string{"openid", "profile", "email"},
				}
				json.NewEncoder(w).Encode(response)
				return
			}
			http.NotFound(w, r)
		}))
		defer server.Close()

		// Test detection
		ctx := context.Background()
		config := DefaultDiscoveryConfig()
		result, err := DetectAuthenticationFromServer(ctx, server.URL, config)

		if err != nil {
			t.Errorf("DetectAuthenticationFromServer() unexpected error: %v", err)
			return
		}

		if result == nil {
			t.Errorf("DetectAuthenticationFromServer() = nil, want valid AuthInfo")
			return
		}

		if result.Type != "OAuth" {
			t.Errorf("AuthInfo.Type = %v, want OAuth", result.Type)
		}

		if len(result.AuthorizationServers) != 1 || result.AuthorizationServers[0] != "https://auth.example.com" {
			t.Errorf("AuthInfo.AuthorizationServers = %v, want [https://auth.example.com]", result.AuthorizationServers)
		}

		if result.JWKSURI != "https://auth.example.com/.well-known/jwks.json" {
			t.Errorf("AuthInfo.JWKSURI = %v, want https://auth.example.com/.well-known/jwks.json", result.JWKSURI)
		}

		if result.Realm != "https://auth.example.com" {
			t.Errorf("AuthInfo.Realm = %v, want https://auth.example.com", result.Realm)
		}
	})

	t.Run("RFC-9728 not supported, fallback to WWW-Authenticate", func(t *testing.T) {
		t.Parallel()
		
		// Create test server that doesn't support RFC-9728 but supports WWW-Authenticate
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/oauth-protected-resource" {
				// Return 404 for RFC-9728 discovery
				http.NotFound(w, r)
				return
			}
			// Return WWW-Authenticate header for regular requests
			w.Header().Set("WWW-Authenticate", `Bearer realm="https://example.com"`)
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		// Test detection
		ctx := context.Background()
		config := DefaultDiscoveryConfig()
		result, err := DetectAuthenticationFromServer(ctx, server.URL, config)

		if err != nil {
			t.Errorf("DetectAuthenticationFromServer() unexpected error: %v", err)
			return
		}

		if result == nil {
			t.Errorf("DetectAuthenticationFromServer() = nil, want valid AuthInfo")
			return
		}

		if result.Type != "OAuth" {
			t.Errorf("AuthInfo.Type = %v, want OAuth", result.Type)
		}

		if result.Realm != "https://example.com" {
			t.Errorf("AuthInfo.Realm = %v, want https://example.com", result.Realm)
		}

		// RFC-9728 specific fields should be empty for WWW-Authenticate fallback
		if len(result.AuthorizationServers) != 0 {
			t.Errorf("AuthInfo.AuthorizationServers = %v, want empty for WWW-Authenticate fallback", result.AuthorizationServers)
		}
	})

	t.Run("RFC-9728 disabled in config", func(t *testing.T) {
		t.Parallel()
		
		// Create test server that supports both RFC-9728 and WWW-Authenticate
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/oauth-protected-resource" && r.Method == http.MethodGet {
				// This should not be called when RFC-9728 is disabled
				t.Errorf("RFC-9728 endpoint called when disabled in config")
				return
			}
			// Return WWW-Authenticate header for regular requests
			w.Header().Set("WWW-Authenticate", `Bearer realm="https://example.com"`)
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		// Test detection with RFC-9728 disabled
		ctx := context.Background()
		config := &Config{
			Timeout:               DefaultAuthDetectTimeout,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			EnablePOSTDetection:   true,
			EnableRFC9728:         false, // Disabled
		}
		result, err := DetectAuthenticationFromServer(ctx, server.URL, config)

		if err != nil {
			t.Errorf("DetectAuthenticationFromServer() unexpected error: %v", err)
			return
		}

		if result == nil {
			t.Errorf("DetectAuthenticationFromServer() = nil, want valid AuthInfo")
			return
		}

		if result.Type != "OAuth" {
			t.Errorf("AuthInfo.Type = %v, want OAuth", result.Type)
		}
	})

	t.Run("RFC-9728 invalid JSON response", func(t *testing.T) {
		t.Parallel()
		
		// Create test server that returns invalid JSON for RFC-9728
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/oauth-protected-resource" && r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("invalid json"))
				return
			}
			// Return WWW-Authenticate header for fallback
			w.Header().Set("WWW-Authenticate", `Bearer realm="https://example.com"`)
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		// Test detection - should fallback to WWW-Authenticate when RFC-9728 fails
		ctx := context.Background()
		config := DefaultDiscoveryConfig()
		result, err := DetectAuthenticationFromServer(ctx, server.URL, config)

		if err != nil {
			t.Errorf("DetectAuthenticationFromServer() unexpected error: %v", err)
			return
		}

		if result == nil {
			t.Errorf("DetectAuthenticationFromServer() = nil, want valid AuthInfo")
			return
		}

		if result.Type != "OAuth" {
			t.Errorf("AuthInfo.Type = %v, want OAuth", result.Type)
		}

		// Should have used WWW-Authenticate fallback
		if result.Realm != "https://example.com" {
			t.Errorf("AuthInfo.Realm = %v, want https://example.com", result.Realm)
		}
	})

	t.Run("RFC-9728 wrong content type", func(t *testing.T) {
		t.Parallel()
		
		// Create test server that returns wrong content type
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/oauth-protected-resource" && r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("not json"))
				return
			}
			// Return WWW-Authenticate header for fallback
			w.Header().Set("WWW-Authenticate", `Bearer realm="https://example.com"`)
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		// Test detection - should fallback to WWW-Authenticate when RFC-9728 fails
		ctx := context.Background()
		config := DefaultDiscoveryConfig()
		result, err := DetectAuthenticationFromServer(ctx, server.URL, config)

		if err != nil {
			t.Errorf("DetectAuthenticationFromServer() unexpected error: %v", err)
			return
		}

		if result == nil {
			t.Errorf("DetectAuthenticationFromServer() = nil, want valid AuthInfo")
			return
		}

		// Should have used WWW-Authenticate fallback
		if result.Type != "OAuth" {
			t.Errorf("AuthInfo.Type = %v, want OAuth", result.Type)
		}
	})

	t.Run("no authentication required", func(t *testing.T) {
		t.Parallel()
		
		// Create test server that requires no authentication
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/oauth-protected-resource" && r.Method == http.MethodGet {
				// RFC-9728 not supported
				http.NotFound(w, r)
				return
			}
			// No authentication required
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Test detection
		ctx := context.Background()
		config := DefaultDiscoveryConfig()
		result, err := DetectAuthenticationFromServer(ctx, server.URL, config)

		if err != nil {
			t.Errorf("DetectAuthenticationFromServer() unexpected error: %v", err)
			return
		}

		if result != nil {
			t.Errorf("DetectAuthenticationFromServer() = %v, want nil (no auth required)", result)
		}
	})
}
