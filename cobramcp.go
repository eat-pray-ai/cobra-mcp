// Copyright 2026 eat-pray-ai & OpenWaygate
// SPDX-License-Identifier: Apache-2.0

// Package cobramcp bridges cobra CLI applications and the Model Context
// Protocol (MCP). It provides a pre-configured MCP server and a cobra
// command that starts the server in stdio or HTTP mode.
package cobramcp

import (
	"context"
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
	mcpShort       = "Start MCP server"
	mcpLong        = "Start MCP server to handle requests from clients"
	modeUsage      = "stdio|http"
	hostUsage      = "Host to bind for HTTP mode"
	portUsage      = "Port to listen on for HTTP mode"
	baseURLUsage   = "Base URL for the MCP server (default http://<host>:<port>)"
	statelessUsage = "Enable stateless HTTP mode (required for MCP 2026-07-28 clients)"

	wellKnownOAuthPath = "/.well-known/oauth-protected-resource"
)

// Config holds the settings used to create a new MCP server and its
// accompanying cobra command.
type Config struct {
	// Name identifies the MCP server implementation (e.g. "yutu").
	Name string

	// Version is the implementation version reported to clients.
	Version string

	// Auth enables MCP OAuth authorization on the HTTP transport.
	// When nil, no authentication is required (backward compatible).
	Auth *AuthConfig

	// HTTPOptions allows overriding the full StreamableHTTP options.
	// When set, the --stateless flag still overrides HTTPOptions.Stateless.
	// Fields like JSONResponse, MaxRequestBodyBytes, and
	// PropagateRequestCancellation can be configured here.
	HTTPOptions *mcp.StreamableHTTPOptions

	// ListCacheTTL sets the TTL hint (in milliseconds) on */list responses.
	// Clients MAY cache these results for this duration. Use this when the
	// server's tool/resource set is static. Zero means no caching hint.
	ListCacheTTL int

	// ServerOptions configures the MCP server. Fields like Instructions,
	// PageSize, and KeepAlive are set here directly.
	// PageSize defaults to 100 if zero; KeepAlive defaults to 13s unless
	// HTTPOptions.Stateless is true (in which case it stays 0).
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
	// When empty, defaults to BaseURL + wellKnownOAuthPath.
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

	var opts mcp.ServerOptions
	if cfg.ServerOptions != nil {
		opts = *cfg.ServerOptions
	}
	if opts.PageSize == 0 {
		opts.PageSize = 100
	}
	if opts.KeepAlive == 0 && !isStateless(cfg) {
		opts.KeepAlive = 13 * time.Second
	}

	server := mcp.NewServer(impl, &opts)
	if cfg.ListCacheTTL > 0 {
		server.AddReceivingMiddleware(listCacheTTLMiddleware(cfg.ListCacheTTL))
	}
	return server
}

func isStateless(cfg *Config) bool {
	return cfg.HTTPOptions != nil && cfg.HTTPOptions.Stateless
}

func listCacheTTLMiddleware(ttl int) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if err != nil {
				return res, err
			}
			switch r := res.(type) {
			case *mcp.ListToolsResult:
				r.TTLMs = ttl
			case *mcp.ListPromptsResult:
				r.TTLMs = ttl
			case *mcp.ListResourcesResult:
				r.TTLMs = ttl
			case *mcp.ListResourceTemplatesResult:
				r.TTLMs = ttl
			}
			return res, err
		}
	}
}

// buildHTTPHandler wraps the MCP streamable-HTTP handler with optional OAuth
// middleware. When cfg.Auth is nil, the raw handler is returned unchanged.
func buildHTTPHandler(cfg *Config, server *mcp.Server, stateless bool) http.Handler {
	var opts mcp.StreamableHTTPOptions
	if cfg.HTTPOptions != nil {
		opts = *cfg.HTTPOptions
	}
	opts.Stateless = stateless
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server }, &opts,
	)
	if cfg.Auth == nil {
		return handler
	}
	mux := http.NewServeMux()
	mux.Handle(
		wellKnownOAuthPath,
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
		mode      string
		host      string
		port      int
		baseURL   string
		stateless bool
	)

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: mcpShort,
		Long:  mcpLong,
		Run: func(cmd *cobra.Command, args []string) {
			var err error
			ctx := cmd.Context()
			addr := fmt.Sprintf("%s:%d", host, port)

			if cfg.Auth != nil {
				resolveAuthDefaults(cfg.Auth, baseURL, addr)
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
				if cfg.Auth == nil {
					slog.WarnContext(
						ctx,
						"no authentication configured; all MCP tools are exposed to any reachable client",
					)
				}
				url := fmt.Sprintf("http://%s/mcp", addr)
				slog.InfoContext(
					ctx, "http server configuration", "url", url, "auth", cfg.Auth != nil,
				)
				httpHandler := buildHTTPHandler(cfg, server, stateless)
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
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", hostUsage)
	cmd.Flags().IntVarP(&port, "port", "p", 8216, portUsage)
	cmd.Flags().StringVarP(&baseURL, "baseUrl", "b", "", baseURLUsage)
	cmd.Flags().BoolVar(&stateless, "stateless", true, statelessUsage)

	return cmd
}

func resolveAuthDefaults(auth *AuthConfig, baseURL, addr string) {
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s", addr)
	}
	if auth.ResourceMetadataURL == "" {
		auth.ResourceMetadataURL = baseURL + wellKnownOAuthPath
	}
	if auth.ResourceMetadata == nil {
		auth.ResourceMetadata = &oauthex.ProtectedResourceMetadata{
			Resource:             baseURL + "/mcp",
			AuthorizationServers: auth.AuthorizationServers,
			ScopesSupported:      auth.Scopes,
		}
	}
}
