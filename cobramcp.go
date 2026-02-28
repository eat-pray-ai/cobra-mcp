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

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	// Defaults to 99 if zero.
	PageSize int

	// KeepAlive sets the interval for server keep-alive pings.
	// Defaults to 13s if zero.
	KeepAlive time.Duration

	// DefaultPort is the default port for HTTP mode.
	// Defaults to 8216 if zero.
	DefaultPort int

	// ServerOptions allows overriding the full MCP server options.
	// When set, Instructions, PageSize, and KeepAlive are ignored.
	ServerOptions *mcp.ServerOptions
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
			pageSize = 99
		}
		keepAlive := cfg.KeepAlive
		if keepAlive == 0 {
			keepAlive = 13 * time.Second
		}

		opts = &mcp.ServerOptions{
			Instructions: cfg.Instructions,
			PageSize:     pageSize,
			KeepAlive:    keepAlive,
			Capabilities: &mcp.ServerCapabilities{
				Logging: &mcp.LoggingCapabilities{},
				Resources: &mcp.ResourceCapabilities{
					ListChanged: true,
					Subscribe:   true,
				},
				Tools: &mcp.ToolCapabilities{
					ListChanged: true,
				},
			},
		}
	}

	return mcp.NewServer(impl, opts)
}

func newCommand(cfg *Config, server *mcp.Server) *cobra.Command {
	var (
		mode string
		port int
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
				handler := mcp.NewStreamableHTTPHandler(
					func(*http.Request) *mcp.Server {
						return server
					}, nil,
				)
				slog.InfoContext(
					ctx, "http server configuration",
					"url", fmt.Sprintf("http://localhost:%d/mcp", port),
				)
				err = http.ListenAndServe(addr, handler)
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

	return cmd
}
