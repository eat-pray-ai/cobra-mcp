// Copyright 2026 eat-pray-ai & OpenWaygate
// SPDX-License-Identifier: Apache-2.0

package cobramcp

import (
	"context"
	"errors"
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

func TestGenResourceHandler_Success(t *testing.T) {
	handler := GenResourceHandler(
		"test-resource", "application/json",
		func(req *mcp.ReadResourceRequest, w io.Writer) error {
			_, _ = io.WriteString(w, `{"uri":"`+req.Params.URI+`"}`)
			return nil
		},
	)

	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "test://example"},
	}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
	c := result.Contents[0]
	if c.URI != "test://example" {
		t.Errorf("URI = %q, want %q", c.URI, "test://example")
	}
	if c.MIMEType != "application/json" {
		t.Errorf("MIMEType = %q, want %q", c.MIMEType, "application/json")
	}
	if c.Text != `{"uri":"test://example"}` {
		t.Errorf("Text = %q, want %q", c.Text, `{"uri":"test://example"}`)
	}
}

func TestGenResourceHandler_Error(t *testing.T) {
	errExpected := errors.New("resource not found")
	handler := GenResourceHandler(
		"test-resource", "text/plain",
		func(req *mcp.ReadResourceRequest, w io.Writer) error {
			return errExpected
		},
	)

	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "test://missing"},
	}
	result, err := handler(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errExpected) {
		t.Errorf("error = %v, want %v", err, errExpected)
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %v", result)
	}
}

func TestGenResourceHandler_EmptyOutput(t *testing.T) {
	handler := GenResourceHandler(
		"empty-resource", "text/plain",
		func(req *mcp.ReadResourceRequest, w io.Writer) error {
			return nil
		},
	)

	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "test://empty"},
	}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	c := result.Contents[0]
	if c.Text != "" {
		t.Errorf("Text = %q, want empty string", c.Text)
	}
}
