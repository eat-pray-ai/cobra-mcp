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

// ContextAware can be implemented by tool input types to receive the request
// context. When a tool input type implements this interface, GenToolHandler
// calls SetContext with the incoming context before invoking the operation.
type ContextAware interface {
	SetContext(context.Context)
}

// GenToolHandler creates a typed MCP tool handler that deserializes JSON input
// into T, calls op, and returns the written output as text content.
func GenToolHandler[T any](
	toolName string, op func(T, io.Writer) error,
) mcp.ToolHandlerFor[T, any] {
	return func(
		ctx context.Context, req *mcp.CallToolRequest, input T,
	) (*mcp.CallToolResult, any, error) {
		var logger *slog.Logger
		if req.Session != nil {
			logger = slog.New(
				mcp.NewLoggingHandler(
					req.Session,
					&mcp.LoggingHandlerOptions{
						LoggerName: toolName, MinInterval: time.Second,
					},
				),
			)
		}

		if ca, ok := any(&input).(ContextAware); ok {
			ca.SetContext(ctx)
		}

		var writer bytes.Buffer
		err := op(input, &writer)

		inputJSON, _ := json.Marshal(input)

		if err != nil {
			if logger != nil {
				logger.ErrorContext(ctx, err.Error(), "input", string(inputJSON))
			}
			slog.ErrorContext(
				ctx, err.Error(), "tool", toolName, "input", string(inputJSON),
			)
			return nil, nil, err
		}

		if logger != nil {
			logger.InfoContext(
				ctx, toolName,
				"input", string(inputJSON), "output_length", writer.Len(),
			)
		}
		slog.InfoContext(
			ctx, toolName,
			"input", string(inputJSON), "output_length", writer.Len(),
		)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: writer.String()}},
		}, nil, nil
	}
}

// GenPromptHandler creates an MCP prompt handler that calls op and returns
// the resulting messages directly.
func GenPromptHandler(
	name string,
	op func(*mcp.GetPromptRequest) ([]*mcp.PromptMessage, error),
) mcp.PromptHandler {
	return func(
		ctx context.Context, req *mcp.GetPromptRequest,
	) (*mcp.GetPromptResult, error) {
		var logger *slog.Logger
		if req.Session != nil {
			logger = slog.New(
				mcp.NewLoggingHandler(
					req.Session,
					&mcp.LoggingHandlerOptions{
						LoggerName: name, MinInterval: time.Second,
					},
				),
			)
		}

		messages, err := op(req)
		if err != nil {
			if logger != nil {
				logger.ErrorContext(ctx, err.Error(), "prompt", name)
			}
			slog.ErrorContext(ctx, err.Error(), "prompt", name)
			return nil, err
		}

		if logger != nil {
			logger.InfoContext(ctx, "prompt get", "prompt", name)
		}
		slog.InfoContext(ctx, "prompt get", "prompt", name)

		return &mcp.GetPromptResult{Messages: messages}, nil
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
		var logger *slog.Logger
		if req.Session != nil {
			logger = slog.New(
				mcp.NewLoggingHandler(
					req.Session,
					&mcp.LoggingHandlerOptions{
						LoggerName: name, MinInterval: time.Second,
					},
				),
			)
		}

		var writer bytes.Buffer
		err := op(req, &writer)
		if err != nil {
			if logger != nil {
				logger.ErrorContext(ctx, err.Error(), "uri", req.Params.URI)
			}
			slog.ErrorContext(ctx, err.Error(), "uri", req.Params.URI)
			return nil, err
		}

		if logger != nil {
			logger.InfoContext(ctx, "resource read", "uri", req.Params.URI)
		}
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
