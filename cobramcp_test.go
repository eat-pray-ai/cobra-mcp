// Copyright 2026 eat-pray-ai & OpenWaygate
// SPDX-License-Identifier: Apache-2.0

package cobramcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

func mockVerifier() auth.TokenVerifier {
	return func(ctx context.Context, token string, req *http.Request) (*auth.TokenInfo, error) {
		if token == "valid-token" {
			return &auth.TokenInfo{
				Scopes:     []string{"read"},
				Expiration: time.Now().Add(time.Hour),
				UserID:     "user-123",
				Extra:      map[string]any{"access_token": token},
			}, nil
		}
		return nil, auth.ErrInvalidToken
	}
}

func TestServerAndCommand(t *testing.T) {
	cfg := &Config{Name: "test-app", Version: "1.0.0", ServerOptions: &mcp.ServerOptions{Instructions: "test instructions"}}
	server, cmd := ServerAndCommand(cfg)

	if server == nil {
		t.Fatal("expected non-nil server")
	}
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Use != "mcp" {
		t.Errorf("cmd.Use = %q, want %q", cmd.Use, "mcp")
	}
}

func TestNewServer(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "defaults",
			cfg:  &Config{Name: "s", Version: "1.0.0"},
		},
		{
			name: "custom server options",
			cfg: &Config{
				Name:    "s",
				Version: "1.0.0",
				ServerOptions: &mcp.ServerOptions{
					Instructions: "custom",
					PageSize:     50,
					KeepAlive:    5 * time.Second,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newServer(tt.cfg)
			if server == nil {
				t.Fatal("expected non-nil server")
			}
		})
	}
}

func TestNewCommand_Flags(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]string
		wantMode  string
		wantHost  string
		wantPort  int
		wantBase  string
		wantSL    bool
	}{
		{
			name:     "defaults",
			wantMode: "stdio",
			wantHost: "127.0.0.1",
			wantPort: 8216,
			wantBase: "",
			wantSL:   true,
		},
		{
			name:      "overridden",
			overrides: map[string]string{"host": "0.0.0.0", "port": "9000", "stateless": "false"},
			wantMode:  "stdio",
			wantHost:  "0.0.0.0",
			wantPort:  9000,
			wantBase:  "",
			wantSL:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Name: "test", Version: "0.1.0"}
			cmd := newCommand(cfg, newServer(cfg))

			for k, v := range tt.overrides {
				if err := cmd.Flags().Set(k, v); err != nil {
					t.Fatalf("setting flag %q: %v", k, err)
				}
			}

			mode, _ := cmd.Flags().GetString("mode")
			host, _ := cmd.Flags().GetString("host")
			port, _ := cmd.Flags().GetInt("port")
			baseURL, _ := cmd.Flags().GetString("baseUrl")
			stateless, _ := cmd.Flags().GetBool("stateless")

			if mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", mode, tt.wantMode)
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
			if baseURL != tt.wantBase {
				t.Errorf("baseUrl = %q, want %q", baseURL, tt.wantBase)
			}
			if stateless != tt.wantSL {
				t.Errorf("stateless = %v, want %v", stateless, tt.wantSL)
			}
		})
	}
}

func TestResolveAuthDefaults(t *testing.T) {
	tests := []struct {
		name            string
		auth            *AuthConfig
		baseURL         string
		addr            string
		wantMetaURL     string
		wantResource    string
		wantAuthServers []string
		wantPreset      bool
	}{
		{
			name: "all empty derives from addr",
			auth: &AuthConfig{
				Scopes:               []string{"read", "write"},
				AuthorizationServers: []string{"https://auth.example.com"},
			},
			baseURL:         "",
			addr:            "127.0.0.1:8216",
			wantMetaURL:     "http://127.0.0.1:8216/.well-known/oauth-protected-resource",
			wantResource:    "http://127.0.0.1:8216/mcp",
			wantAuthServers: []string{"https://auth.example.com"},
		},
		{
			name: "explicit baseURL used",
			auth: &AuthConfig{
				Scopes:               []string{"read"},
				AuthorizationServers: []string{"https://auth.example.com"},
			},
			baseURL:         "https://mcp.example.com",
			addr:            "0.0.0.0:8216",
			wantMetaURL:     "https://mcp.example.com/.well-known/oauth-protected-resource",
			wantResource:    "https://mcp.example.com/mcp",
			wantAuthServers: []string{"https://auth.example.com"},
		},
		{
			name: "preset values not overwritten",
			auth: &AuthConfig{
				ResourceMetadata:    &oauthex.ProtectedResourceMetadata{Resource: "http://custom/mcp"},
				ResourceMetadataURL: "http://custom/.well-known/oauth-protected-resource",
				TokenVerifier:       mockVerifier(),
				Scopes:              []string{"read"},
			},
			baseURL:     "http://should-not-override",
			addr:        "127.0.0.1:9999",
			wantMetaURL: "http://custom/.well-known/oauth-protected-resource",
			wantPreset:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset := tt.auth.ResourceMetadata
			resolveAuthDefaults(tt.auth, tt.baseURL, tt.addr)

			if tt.auth.ResourceMetadataURL != tt.wantMetaURL {
				t.Errorf("ResourceMetadataURL = %q, want %q", tt.auth.ResourceMetadataURL, tt.wantMetaURL)
			}
			if tt.wantPreset {
				if tt.auth.ResourceMetadata != preset {
					t.Error("ResourceMetadata was overwritten")
				}
				return
			}
			if tt.auth.ResourceMetadata == nil {
				t.Fatal("expected ResourceMetadata to be auto-constructed")
			}
			if tt.auth.ResourceMetadata.Resource != tt.wantResource {
				t.Errorf("Resource = %q, want %q", tt.auth.ResourceMetadata.Resource, tt.wantResource)
			}
			if len(tt.auth.ResourceMetadata.AuthorizationServers) != len(tt.wantAuthServers) {
				t.Fatalf("AuthorizationServers = %v, want %v", tt.auth.ResourceMetadata.AuthorizationServers, tt.wantAuthServers)
			}
			for i, s := range tt.auth.ResourceMetadata.AuthorizationServers {
				if s != tt.wantAuthServers[i] {
					t.Errorf("AuthorizationServers[%d] = %q, want %q", i, s, tt.wantAuthServers[i])
				}
			}
		})
	}
}

func authConfig() *AuthConfig {
	return &AuthConfig{
		ResourceMetadata: &oauthex.ProtectedResourceMetadata{
			Resource:             "http://localhost:8216/mcp",
			AuthorizationServers: []string{"https://accounts.google.com"},
			ScopesSupported:      []string{"read"},
		},
		ResourceMetadataURL: "http://localhost:8216/.well-known/oauth-protected-resource",
		TokenVerifier:       mockVerifier(),
		Scopes:              []string{"read"},
	}
}

func TestHTTPMode_Auth(t *testing.T) {
	tests := []struct {
		name       string
		auth       *AuthConfig
		method     string
		path       string
		token      string
		wantStatus int
		checkBody  func(t *testing.T, rr *httptest.ResponseRecorder)
	}{
		{
			name:       "no auth config allows requests",
			auth:       nil,
			method:     "POST",
			path:       "/mcp",
			wantStatus: -1, // any non-401
		},
		{
			name:       "unauthenticated returns 401",
			auth:       authConfig(),
			method:     "POST",
			path:       "/mcp",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid token returns 401",
			auth:       authConfig(),
			method:     "POST",
			path:       "/mcp",
			token:      "invalid-token",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "valid token passes",
			auth:       authConfig(),
			method:     "POST",
			path:       "/mcp",
			token:      "valid-token",
			wantStatus: -1, // any non-401
		},
		{
			name:       "metadata endpoint returns resource info",
			auth:       authConfig(),
			method:     "GET",
			path:       "/.well-known/oauth-protected-resource",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var got oauthex.ProtectedResourceMetadata
				if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
					t.Fatalf("failed to decode metadata: %v", err)
				}
				if got.Resource != "http://localhost:8216/mcp" {
					t.Errorf("resource = %q, want %q", got.Resource, "http://localhost:8216/mcp")
				}
				if len(got.AuthorizationServers) != 1 || got.AuthorizationServers[0] != "https://accounts.google.com" {
					t.Errorf("authorization_servers = %v", got.AuthorizationServers)
				}
			},
		},
		{
			name:       "no auth does not serve well-known",
			auth:       nil,
			method:     "GET",
			path:       "/.well-known/oauth-protected-resource",
			wantStatus: -2, // not 200
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Name: "test", Version: "0.1.0", Auth: tt.auth}
			server := newServer(cfg)
			mcp.AddTool(server, &mcp.Tool{Name: "echo"}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
			})
			handler := buildHTTPHandler(cfg, server, false)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			switch tt.wantStatus {
			case -1: // not 401
				if rr.Code == http.StatusUnauthorized {
					t.Errorf("got 401, expected non-401")
				}
			case -2: // not 200
				if rr.Code == http.StatusOK {
					t.Errorf("got 200, expected non-200")
				}
			default:
				if rr.Code != tt.wantStatus {
					t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
				}
			}

			if tt.checkBody != nil {
				tt.checkBody(t, rr)
			}
		})
	}
}

func TestHTTPMode_Stateless(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       string
		wantStatus int
		noSession  bool
	}{
		{
			name:       "GET rejected",
			method:     "GET",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "DELETE rejected",
			method:     "DELETE",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "POST accepted",
			method:     "POST",
			body:       `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "no session header on POST",
			method:     "POST",
			body:       `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`,
			wantStatus: http.StatusOK,
			noSession:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Name: "test", Version: "0.1.0"}
			server := newServer(cfg)
			mcp.AddTool(server, &mcp.Tool{Name: "echo"}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
			})
			handler := buildHTTPHandler(cfg, server, true)

			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, "/mcp", bytes.NewBufferString(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, "/mcp", nil)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rr.Code, tt.wantStatus, rr.Body.String())
			}
			if tt.noSession {
				if sid := rr.Header().Get("Mcp-Session-Id"); sid != "" {
					t.Errorf("expected no Mcp-Session-Id, got %q", sid)
				}
			}
		})
	}
}

func TestBuildHTTPHandler_HTTPOptions(t *testing.T) {
	tests := []struct {
		name       string
		opts       *mcp.StreamableHTTPOptions
		stateless  bool
		method     string
		wantStatus int
	}{
		{
			name:       "custom options with stateless override",
			opts:       &mcp.StreamableHTTPOptions{MaxRequestBodyBytes: 1024},
			stateless:  true,
			method:     "GET",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "stateless flag overrides HTTPOptions.Stateless=false",
			opts:       &mcp.StreamableHTTPOptions{Stateless: false},
			stateless:  true,
			method:     "GET",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "nil options defaults work",
			opts:       nil,
			stateless:  true,
			method:     "DELETE",
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Name: "test", Version: "0.1.0", HTTPOptions: tt.opts}
			server := newServer(cfg)
			handler := buildHTTPHandler(cfg, server, tt.stateless)

			req := httptest.NewRequest(tt.method, "/mcp", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}