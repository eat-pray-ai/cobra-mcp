// Copyright 2026 eat-pray-ai & OpenWaygate
// SPDX-License-Identifier: Apache-2.0

package cobramcp

import (
	"context"
	"io"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ctxKey struct{}

type contextAwareInput struct {
	Name string `json:"name"`
	ctx  context.Context
}

func (c *contextAwareInput) SetContext(ctx context.Context) {
	c.ctx = ctx
}

func TestGenToolHandler_InjectsContext(t *testing.T) {
	handler := GenToolHandler(
		"test-tool",
		func(input contextAwareInput, w io.Writer) error {
			val := input.ctx.Value(ctxKey{})
			if val == nil {
				t.Error("expected context value to be set")
				return nil
			}
			if val != "injected" {
				t.Errorf("context value = %v, want 'injected'", val)
			}
			return nil
		},
	)

	ctx := context.WithValue(context.Background(), ctxKey{}, "injected")
	result, _, err := handler(ctx, &mcp.CallToolRequest{}, contextAwareInput{Name: "test"})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

type plainInput struct {
	Name string `json:"name"`
}

func TestGenToolHandler_PlainInput_NoError(t *testing.T) {
	handler := GenToolHandler(
		"test-tool",
		func(input plainInput, w io.Writer) error {
			_, _ = io.WriteString(w, "hello "+input.Name)
			return nil
		},
	)

	result, _, err := handler(context.Background(), &mcp.CallToolRequest{}, plainInput{Name: "world"})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if text != "hello world" {
		t.Errorf("text = %q, want 'hello world'", text)
	}
}
