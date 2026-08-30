// Copyright 2026 eat-pray-ai & OpenWaygate
// SPDX-License-Identifier: Apache-2.0

package cobramcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"

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
		ctx context.Context, _ *mcp.CallToolRequest, input T,
	) (*mcp.CallToolResult, any, error) {
		if ca, ok := any(&input).(ContextAware); ok {
			ca.SetContext(ctx)
		}

		var writer bytes.Buffer
		err := op(input, &writer)

		inputJSON, _ := json.Marshal(input)

		if err != nil {
			slog.ErrorContext(
				ctx, err.Error(), "tool", toolName, "input", string(inputJSON),
			)
			return nil, nil, err
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

// GenToolHandlerWithMRTR creates a typed MCP tool handler that supports
// multi round-trip requests (MRTR). Unlike GenToolHandler, the op receives
// the full CallToolRequest so it can inspect InputResponses and return a
// *CallToolResult directly (e.g. with InputRequests set for elicitation).
// Return (nil, nil) from op to use the buffer content as a normal text
// response.
func GenToolHandlerWithMRTR[T any](
	toolName string,
	op func(context.Context, *mcp.CallToolRequest, T, io.Writer) (*mcp.CallToolResult, error),
) mcp.ToolHandlerFor[T, any] {
	return func(
		ctx context.Context, req *mcp.CallToolRequest, input T,
	) (*mcp.CallToolResult, any, error) {
		if ca, ok := any(&input).(ContextAware); ok {
			ca.SetContext(ctx)
		}

		var writer bytes.Buffer
		result, err := op(ctx, req, input, &writer)

		inputJSON, _ := json.Marshal(input)

		if err != nil {
			slog.ErrorContext(
				ctx, err.Error(), "tool", toolName, "input", string(inputJSON),
			)
			return nil, nil, err
		}

		if result != nil {
			slog.InfoContext(
				ctx, toolName, "input", string(inputJSON), "mrtr", true,
			)
			return result, nil, nil
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

// ConfirmThen wraps a simple operation with a single-round confirmation
// elicitation. On the first call (no InputResponses), it returns an
// ElicitParams with the given message. On retry with "accept", it executes
// the op. On "decline" or missing response, it returns a cancellation result.
//
// Usage:
//
//	GenToolHandlerWithMRTR("video-delete", ConfirmThen(msgFn, deleteFn))
func ConfirmThen[T any](
	msg func(T) string,
	op func(T, io.Writer) error,
) func(context.Context, *mcp.CallToolRequest, T, io.Writer) (*mcp.CallToolResult, error) {
	return func(
		_ context.Context, req *mcp.CallToolRequest, input T, w io.Writer,
	) (*mcp.CallToolResult, error) {
		if req.Params.InputResponses == nil {
			return &mcp.CallToolResult{
				InputRequests: mcp.InputRequestMap{
					"confirm": &mcp.ElicitParams{
						Message: msg(input),
					},
				},
				RequestState: "awaiting_confirm",
			}, nil
		}

		resp, ok := req.Params.InputResponses["confirm"].(*mcp.ElicitResult)
		if !ok || resp.Action != "accept" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "canceled by user"},
				},
				IsError: true,
			}, nil
		}

		if err := op(input, w); err != nil {
			return nil, err
		}
		return nil, nil
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
		messages, err := op(req)
		if err != nil {
			slog.ErrorContext(ctx, err.Error(), "prompt", name)
			return nil, err
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
		var writer bytes.Buffer
		err := op(req, &writer)
		if err != nil {
			slog.ErrorContext(ctx, err.Error(), "uri", req.Params.URI)
			return nil, err
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
