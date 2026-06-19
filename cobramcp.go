// Copyright 2026 eat-pray-ai & OpenWaygate
// SPDX-License-Identifier: Apache-2.0

// Package cobramcp bridges cobra CLI applications and the Model Context
// Protocol (MCP). It provides a pre-configured MCP server and a cobra
// command that starts the server in stdio or HTTP mode.
package cobramcp

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/spf13/cobra"
)

const (
	mcpShort  = "Start MCP server"
	mcpLong   = "Start MCP server to handle requests from clients"
	modeUsage = "stdio|http"
	portUsage = "Port to listen on for HTTP mode"
)

// Config holds the settings used to create a new MCP server and its
// accompanying cobra command.
type Config struct {
	// Name identifies the MCP server implementation (e.g. "yutu").
	Name string

	// Version is the implementation version reported to clients.
	Version string

	// Instructions provides a brief description of the server's purpose.
	Instructions string

	// PageSize controls the pagination size for list operations.
	// Defaults to 100 if zero.
	PageSize int

	// KeepAlive sets the interval for server keep-alive pings.
	// Defaults to 13s if zero.
	KeepAlive time.Duration

	// DefaultPort is the default port for HTTP mode.
	// Defaults to 8216 if zero.
	DefaultPort int

	// Auth enables MCP OAuth authorization on the HTTP transport.
	// When nil, no authentication is required (backward compatible).
	Auth *AuthConfig

	// ServerOptions allows overriding the full MCP server options.
	// When set, Instructions, PageSize, and KeepAlive are ignored.
	ServerOptions *mcp.ServerOptions
}

// AuthConfig enables MCP OAuth authorization on HTTP transport.
type AuthConfig struct {
	// ResourceMetadata is the OAuth 2.0 Protected Resource Metadata (RFC 9728)
	// served at the well-known endpoint.
	// When nil, auto-constructed from BaseURL, AuthorizationServers, and Scopes.
	ResourceMetadata *oauthex.ProtectedResourceMetadata

	// ResourceMetadataURL is the URL returned in WWW-Authenticate headers
	// so clients can discover the resource metadata.
	// When empty, defaults to BaseURL + "/.well-known/oauth-protected-resource".
	ResourceMetadataURL string

	// TokenVerifier validates Bearer tokens on incoming requests.
	TokenVerifier auth.TokenVerifier

	// Scopes are the required OAuth scopes for accessing the MCP endpoint.
	Scopes []string

	// AuthorizationServers lists the OAuth 2.0 authorization server URLs.
	// Used when auto-constructing ResourceMetadata.
	AuthorizationServers []string
}

// ServerAndCommand creates a new MCP server and a cobra command that starts
// it. The caller registers tools and resources on the returned server, then
// adds the returned command to their root cobra command.
func ServerAndCommand(cfg *Config) (*mcp.Server, *cobra.Command) {
	server := newServer(cfg)
	cmd := newCommand(cfg, server)
	return server, cmd
}

func newServer(cfg *Config) *mcp.Server {
	impl := &mcp.Implementation{
		Name:    cfg.Name,
		Version: cfg.Version,
	}

	opts := cfg.ServerOptions
	if opts == nil {
		pageSize := cfg.PageSize
		if pageSize == 0 {
			pageSize = 100
		}
		keepAlive := cfg.KeepAlive
		if keepAlive == 0 {
			keepAlive = 13 * time.Second
		}

		opts = &mcp.ServerOptions{
			Instructions: cfg.Instructions,
			PageSize:     pageSize,
			KeepAlive:    keepAlive,
		}
	}

	return mcp.NewServer(impl, opts)
}

// buildHTTPHandler wraps the MCP streamable-HTTP handler with optional OAuth
// middleware. When cfg.Auth is nil, the raw handler is returned unchanged.
func buildHTTPHandler(cfg *Config, server *mcp.Server) http.Handler {
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server }, nil,
	)
	if cfg.Auth == nil {
		return handler
	}
	mux := http.NewServeMux()
	mux.Handle(
		"/.well-known/oauth-protected-resource",
		auth.ProtectedResourceMetadataHandler(cfg.Auth.ResourceMetadata),
	)
	authMiddleware := auth.RequireBearerToken(
		cfg.Auth.TokenVerifier,
		&auth.RequireBearerTokenOptions{
			Scopes:              cfg.Auth.Scopes,
			ResourceMetadataURL: cfg.Auth.ResourceMetadataURL,
		},
	)
	mux.Handle("/mcp", authMiddleware(handler))
	return mux
}

func newCommand(cfg *Config, server *mcp.Server) *cobra.Command {
	var (
		mode    string
		port    int
		baseURL string
	)

	defaultPort := cfg.DefaultPort
	if defaultPort == 0 {
		defaultPort = 8216
	}

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: mcpShort,
		Long:  mcpLong,
		Run: func(cmd *cobra.Command, args []string) {
			var err error
			ctx := cmd.Context()
			addr := fmt.Sprintf(":%d", port)

			if cfg.Auth != nil {
				resolveAuthDefaults(cfg.Auth, baseURL, port)
			}

			slog.InfoContext(
				ctx, "starting MCP server",
				"mode", mode,
				"version", cfg.Version,
			)

			switch mode {
			case "stdio":
				t := &mcp.LoggingTransport{
					Transport: &mcp.StdioTransport{},
					Writer:    os.Stderr,
				}
				err = server.Run(ctx, t)
			case "http":
				httpHandler := buildHTTPHandler(cfg, server)
				slog.InfoContext(
					ctx, "http server configuration",
					"url", fmt.Sprintf("http://localhost:%d/mcp", port),
					"auth", cfg.Auth != nil,
				)
				err = http.ListenAndServe(addr, httpHandler)
			default:
				slog.ErrorContext(
					ctx, "invalid mode",
					"mode", mode, "valid_modes", "stdio, http",
				)
				os.Exit(1)
			}

			if err != nil {
				slog.ErrorContext(
					ctx, "starting server failed",
					"error", err, "mode", mode,
				)
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringVarP(&mode, "mode", "m", "stdio", modeUsage)
	cmd.Flags().IntVarP(&port, "port", "p", defaultPort, portUsage)
	cmd.Flags().StringVarP(
		&baseURL, "baseUrl", "b", "",
		"Base URL for the MCP server (default http://localhost:<port>)",
	)

	return cmd
}

func resolveAuthDefaults(auth *AuthConfig, baseURL string, port int) {
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://localhost:%d", port)
	}
	if auth.ResourceMetadataURL == "" {
		auth.ResourceMetadataURL = baseURL + "/.well-known/oauth-protected-resource"
	}
	if auth.ResourceMetadata == nil {
		auth.ResourceMetadata = &oauthex.ProtectedResourceMetadata{
			Resource:             baseURL + "/mcp",
			AuthorizationServers: auth.AuthorizationServers,
			ScopesSupported:      auth.Scopes,
		}
	}
}
