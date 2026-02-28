// Copyright 2026 eat-pray-ai & OpenWaygate
// SPDX-License-Identifier: Apache-2.0

package cobramcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GenToolHandler creates a typed MCP tool handler that deserializes JSON input
// into T, calls op, and returns the written output as text content.
func GenToolHandler[T any](
	toolName string, op func(T, io.Writer) error,
) mcp.ToolHandlerFor[T, any] {
	return func(
		ctx context.Context, req *mcp.CallToolRequest, input T,
	) (*mcp.CallToolResult, any, error) {
		logger := slog.New(
			mcp.NewLoggingHandler(
				req.Session,
				&mcp.LoggingHandlerOptions{
					LoggerName: toolName, MinInterval: time.Second,
				},
			),
		)

		var writer bytes.Buffer
		err := op(input, &writer)

		inputJSON, _ := json.Marshal(input)

		if err != nil {
			logger.ErrorContext(ctx, err.Error(), "input", string(inputJSON))
			slog.ErrorContext(
				ctx, err.Error(), "tool", toolName, "input", string(inputJSON),
			)
			return nil, nil, err
		}

		logger.InfoContext(
			ctx, toolName,
			"input", string(inputJSON), "output_length", writer.Len(),
		)
		slog.InfoContext(
			ctx, toolName,
			"input", string(inputJSON), "output_length", writer.Len(),
		)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: writer.String()}},
		}, nil, nil
	}
}

// GenResourceHandler creates an MCP resource handler that calls op and returns
// the written output as a JSON resource.
func GenResourceHandler(
	name string, mimeType string,
	op func(*mcp.ReadResourceRequest, io.Writer) error,
) mcp.ResourceHandler {
	return func(
		ctx context.Context, req *mcp.ReadResourceRequest,
	) (*mcp.ReadResourceResult, error) {
		logger := slog.New(
			mcp.NewLoggingHandler(
				req.Session,
				&mcp.LoggingHandlerOptions{
					LoggerName: name, MinInterval: time.Second,
				},
			),
		)

		var writer bytes.Buffer
		err := op(req, &writer)
		if err != nil {
			logger.ErrorContext(ctx, err.Error(), "uri", req.Params.URI)
			slog.ErrorContext(ctx, err.Error(), "uri", req.Params.URI)
			return nil, err
		}

		logger.InfoContext(ctx, "resource read", "uri", req.Params.URI)
		slog.InfoContext(
			ctx, "resource read", "resource", name, "uri", req.Params.URI,
		)

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: mimeType, Text: writer.String()},
			},
		}, nil
	}
}
