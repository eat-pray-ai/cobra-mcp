// Copyright 2026 eat-pray-ai & OpenWaygate
// SPDX-License-Identifier: Apache-2.0

package cobramcp

import (
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

func TestHTTPMode_NoAuth_BackwardCompat(t *testing.T) {
	cfg := &Config{Name: "test", Version: "0.1.0"}
	server := newServer(cfg)
	handler := buildHTTPHandler(cfg, server)

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
	handler := buildHTTPHandler(cfg, server)

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
	handler := buildHTTPHandler(cfg, server)

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
	handler := buildHTTPHandler(cfg, server)

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
