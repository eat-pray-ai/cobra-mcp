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
	cfg := &Config{Name: "test-app", Version: "1.0.0", Instructions: "test instructions"}
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

func TestNewServer_Defaults(t *testing.T) {
	cfg := &Config{Name: "s", Version: "1.0.0"}
	server := newServer(cfg)
	if server == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestNewServer_CustomOptions(t *testing.T) {
	custom := &mcp.ServerOptions{
		Instructions: "custom",
		PageSize:     50,
		KeepAlive:    5 * time.Second,
	}
	cfg := &Config{Name: "s", Version: "1.0.0", ServerOptions: custom}
	server := newServer(cfg)
	if server == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestNewCommand_DefaultFlags(t *testing.T) {
	cfg := &Config{Name: "test", Version: "0.1.0"}
	server := newServer(cfg)
	cmd := newCommand(cfg, server)

	mode, _ := cmd.Flags().GetString("mode")
	if mode != "stdio" {
		t.Errorf("default mode = %q, want %q", mode, "stdio")
	}

	host, _ := cmd.Flags().GetString("host")
	if host != "127.0.0.1" {
		t.Errorf("default host = %q, want %q", host, "127.0.0.1")
	}

	port, _ := cmd.Flags().GetInt("port")
	if port != 8216 {
		t.Errorf("default port = %d, want %d", port, 8216)
	}

	baseURL, _ := cmd.Flags().GetString("baseUrl")
	if baseURL != "" {
		t.Errorf("default baseUrl = %q, want empty", baseURL)
	}
}

func TestNewCommand_FlagOverrides(t *testing.T) {
	cfg := &Config{Name: "test", Version: "0.1.0"}
	server := newServer(cfg)
	cmd := newCommand(cfg, server)

	cmd.Flags().Set("host", "0.0.0.0")
	cmd.Flags().Set("port", "9000")

	host, _ := cmd.Flags().GetString("host")
	if host != "0.0.0.0" {
		t.Errorf("overridden host = %q, want %q", host, "0.0.0.0")
	}

	port, _ := cmd.Flags().GetInt("port")
	if port != 9000 {
		t.Errorf("overridden port = %d, want %d", port, 9000)
	}
}

func TestResolveAuthDefaults_AllEmpty(t *testing.T) {
	ac := &AuthConfig{
		Scopes:               []string{"read", "write"},
		AuthorizationServers: []string{"https://auth.example.com"},
	}
	resolveAuthDefaults(ac, "", "127.0.0.1:8216")

	wantMetaURL := "http://127.0.0.1:8216/.well-known/oauth-protected-resource"
	if ac.ResourceMetadataURL != wantMetaURL {
		t.Errorf("ResourceMetadataURL = %q, want %q", ac.ResourceMetadataURL, wantMetaURL)
	}
	if ac.ResourceMetadata == nil {
		t.Fatal("expected ResourceMetadata to be auto-constructed")
	}
	if ac.ResourceMetadata.Resource != "http://127.0.0.1:8216/mcp" {
		t.Errorf("Resource = %q, want %q", ac.ResourceMetadata.Resource, "http://127.0.0.1:8216/mcp")
	}
	if len(ac.ResourceMetadata.AuthorizationServers) != 1 || ac.ResourceMetadata.AuthorizationServers[0] != "https://auth.example.com" {
		t.Errorf("AuthorizationServers = %v", ac.ResourceMetadata.AuthorizationServers)
	}
	if len(ac.ResourceMetadata.ScopesSupported) != 2 {
		t.Errorf("ScopesSupported = %v", ac.ResourceMetadata.ScopesSupported)
	}
}

func TestResolveAuthDefaults_WithBaseURL(t *testing.T) {
	ac := &AuthConfig{
		Scopes:               []string{"read"},
		AuthorizationServers: []string{"https://auth.example.com"},
	}
	resolveAuthDefaults(ac, "https://mcp.example.com", "0.0.0.0:8216")

	wantMetaURL := "https://mcp.example.com/.well-known/oauth-protected-resource"
	if ac.ResourceMetadataURL != wantMetaURL {
		t.Errorf("ResourceMetadataURL = %q, want %q", ac.ResourceMetadataURL, wantMetaURL)
	}
	if ac.ResourceMetadata.Resource != "https://mcp.example.com/mcp" {
		t.Errorf("Resource = %q, want %q", ac.ResourceMetadata.Resource, "https://mcp.example.com/mcp")
	}
}

func TestResolveAuthDefaults_PresetValues(t *testing.T) {
	preset := &oauthex.ProtectedResourceMetadata{
		Resource: "http://custom/mcp",
	}
	ac := &AuthConfig{
		ResourceMetadata:    preset,
		ResourceMetadataURL: "http://custom/.well-known/oauth-protected-resource",
		TokenVerifier:       mockVerifier(),
		Scopes:              []string{"read"},
	}
	resolveAuthDefaults(ac, "http://should-not-override", "127.0.0.1:9999")

	if ac.ResourceMetadataURL != "http://custom/.well-known/oauth-protected-resource" {
		t.Errorf("ResourceMetadataURL was overwritten: %q", ac.ResourceMetadataURL)
	}
	if ac.ResourceMetadata != preset {
		t.Error("ResourceMetadata was overwritten")
	}
}

func TestHTTPMode_NoAuth_BackwardCompat(t *testing.T) {
	cfg := &Config{Name: "test", Version: "0.1.0"}
	server := newServer(cfg)
	handler := buildHTTPHandler(cfg, server, false)

	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusUnauthorized {
		t.Errorf("expected no 401 without auth config, got %d", rr.Code)
	}
}

func TestHTTPMode_Auth_Unauthenticated(t *testing.T) {
	cfg := &Config{
		Name: "test", Version: "0.1.0",
		Auth: &AuthConfig{
			ResourceMetadata: &oauthex.ProtectedResourceMetadata{
				Resource:             "http://localhost:8216/mcp",
				AuthorizationServers: []string{"https://accounts.google.com"},
				ScopesSupported:      []string{"read"},
			},
			ResourceMetadataURL: "http://localhost:8216/.well-known/oauth-protected-resource",
			TokenVerifier:       mockVerifier(),
			Scopes:              []string{"read"},
		},
	}
	server := newServer(cfg)
	handler := buildHTTPHandler(cfg, server, false)

	req := httptest.NewRequest("POST", "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
	if rr.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header")
	}
}

func TestHTTPMode_Auth_InvalidToken(t *testing.T) {
	cfg := &Config{
		Name: "test", Version: "0.1.0",
		Auth: &AuthConfig{
			ResourceMetadata: &oauthex.ProtectedResourceMetadata{
				Resource:             "http://localhost:8216/mcp",
				AuthorizationServers: []string{"https://accounts.google.com"},
				ScopesSupported:      []string{"read"},
			},
			ResourceMetadataURL: "http://localhost:8216/.well-known/oauth-protected-resource",
			TokenVerifier:       mockVerifier(),
			Scopes:              []string{"read"},
		},
	}
	server := newServer(cfg)
	handler := buildHTTPHandler(cfg, server, false)

	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", rr.Code)
	}
}

func TestHTTPMode_Auth_MetadataEndpoint(t *testing.T) {
	metadata := &oauthex.ProtectedResourceMetadata{
		Resource:             "http://localhost:8216/mcp",
		AuthorizationServers: []string{"https://accounts.google.com"},
		ScopesSupported:      []string{"read"},
	}
	cfg := &Config{
		Name: "test", Version: "0.1.0",
		Auth: &AuthConfig{
			ResourceMetadata:    metadata,
			ResourceMetadataURL: "http://localhost:8216/.well-known/oauth-protected-resource",
			TokenVerifier:       mockVerifier(),
			Scopes:              []string{"read"},
		},
	}
	server := newServer(cfg)
	handler := buildHTTPHandler(cfg, server, false)

	req := httptest.NewRequest("GET", "/.well-known/oauth-protected-resource", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var got oauthex.ProtectedResourceMetadata
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode metadata: %v", err)
	}
	if got.Resource != metadata.Resource {
		t.Errorf("resource = %q, want %q", got.Resource, metadata.Resource)
	}
	if len(got.AuthorizationServers) != 1 || got.AuthorizationServers[0] != "https://accounts.google.com" {
		t.Errorf("authorization_servers = %v, want [https://accounts.google.com]", got.AuthorizationServers)
	}
}

func TestHTTPMode_Auth_ValidToken(t *testing.T) {
	cfg := &Config{
		Name: "test", Version: "0.1.0",
		Auth: &AuthConfig{
			ResourceMetadata: &oauthex.ProtectedResourceMetadata{
				Resource:             "http://localhost:8216/mcp",
				AuthorizationServers: []string{"https://accounts.google.com"},
				ScopesSupported:      []string{"read"},
			},
			ResourceMetadataURL: "http://localhost:8216/.well-known/oauth-protected-resource",
			TokenVerifier:       mockVerifier(),
			Scopes:              []string{"read"},
		},
	}
	server := newServer(cfg)
	mcp.AddTool(server, &mcp.Tool{Name: "echo"}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})
	handler := buildHTTPHandler(cfg, server, false)

	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusUnauthorized {
		t.Errorf("expected authenticated request to pass, got 401")
	}
}

func TestBuildHTTPHandler_NoAuth_ReturnsPlainHandler(t *testing.T) {
	cfg := &Config{Name: "test", Version: "0.1.0"}
	server := newServer(cfg)
	handler := buildHTTPHandler(cfg, server, false)

	req := httptest.NewRequest("GET", "/.well-known/oauth-protected-resource", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Error("plain handler should not serve well-known endpoint")
	}
}

func TestHTTPMode_Stateless_RejectsGET(t *testing.T) {
	cfg := &Config{Name: "test", Version: "0.1.0"}
	server := newServer(cfg)
	handler := buildHTTPHandler(cfg, server, true)

	req := httptest.NewRequest("GET", "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("stateless mode GET: expected 405, got %d", rr.Code)
	}
}

func TestHTTPMode_Stateless_RejectsDELETE(t *testing.T) {
	cfg := &Config{Name: "test", Version: "0.1.0"}
	server := newServer(cfg)
	handler := buildHTTPHandler(cfg, server, true)

	req := httptest.NewRequest("DELETE", "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("stateless mode DELETE: expected 405, got %d", rr.Code)
	}
}

func TestHTTPMode_Stateless_AcceptsPOST(t *testing.T) {
	cfg := &Config{Name: "test", Version: "0.1.0"}
	server := newServer(cfg)
	mcp.AddTool(server, &mcp.Tool{Name: "echo"}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})
	handler := buildHTTPHandler(cfg, server, true)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`
	req := httptest.NewRequest("POST", "/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("stateless mode POST: expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestHTTPMode_Stateless_NoSessionHeader(t *testing.T) {
	cfg := &Config{Name: "test", Version: "0.1.0"}
	server := newServer(cfg)
	handler := buildHTTPHandler(cfg, server, true)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`
	req := httptest.NewRequest("POST", "/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if sessionID := rr.Header().Get("Mcp-Session-Id"); sessionID != "" {
		t.Errorf("stateless mode should not return Mcp-Session-Id, got %q", sessionID)
	}
}

func TestBuildHTTPHandler_HTTPOptions_Respected(t *testing.T) {
	cfg := &Config{
		Name:    "test",
		Version: "0.1.0",
		HTTPOptions: &mcp.StreamableHTTPOptions{
			MaxRequestBodyBytes: 1024,
		},
	}
	server := newServer(cfg)
	handler := buildHTTPHandler(cfg, server, true)

	// Verify the handler works (stateless override applies)
	req := httptest.NewRequest("GET", "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 (stateless override), got %d", rr.Code)
	}
}

func TestBuildHTTPHandler_HTTPOptions_StatelessOverride(t *testing.T) {
	cfg := &Config{
		Name:    "test",
		Version: "0.1.0",
		HTTPOptions: &mcp.StreamableHTTPOptions{
			Stateless: false, // will be overridden by the stateless param
		},
	}
	server := newServer(cfg)
	handler := buildHTTPHandler(cfg, server, true)

	req := httptest.NewRequest("GET", "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("stateless flag should override HTTPOptions.Stateless, got %d", rr.Code)
	}
}

func TestNewCommand_StatelessFlag(t *testing.T) {
	cfg := &Config{Name: "test", Version: "0.1.0"}
	server := newServer(cfg)
	cmd := newCommand(cfg, server)

	stateless, _ := cmd.Flags().GetBool("stateless")
	if !stateless {
		t.Error("default stateless should be true")
	}
}
