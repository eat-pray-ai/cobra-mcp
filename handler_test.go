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

type plainInput struct {
	Name string `json:"name"`
}

func TestGenToolHandler(t *testing.T) {
	errBoom := errors.New("boom")

	tests := []struct {
		name     string
		run      func(t *testing.T)
	}{
		{
			name: "injects context into ContextAware input",
			run: func(t *testing.T) {
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
			},
		},
		{
			name: "plain input returns text content",
			run: func(t *testing.T) {
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
					t.Errorf("text = %q, want %q", text, "hello world")
				}
			},
		},
		{
			name: "op error is propagated",
			run: func(t *testing.T) {
				handler := GenToolHandler(
					"test-tool",
					func(input plainInput, w io.Writer) error {
						return errBoom
					},
				)

				_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, plainInput{Name: "x"})
				if !errors.Is(err, errBoom) {
					t.Errorf("error = %v, want %v", err, errBoom)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestGenResourceHandler(t *testing.T) {
	errNotFound := errors.New("resource not found")

	tests := []struct {
		name     string
		resName  string
		mimeType string
		op       func(*mcp.ReadResourceRequest, io.Writer) error
		uri      string
		wantErr  error
		wantText string
	}{
		{
			name:     "success with JSON output",
			resName:  "test-resource",
			mimeType: "application/json",
			op: func(req *mcp.ReadResourceRequest, w io.Writer) error {
				_, _ = io.WriteString(w, `{"uri":"`+req.Params.URI+`"}`)
				return nil
			},
			uri:      "test://example",
			wantText: `{"uri":"test://example"}`,
		},
		{
			name:     "op returns error",
			resName:  "test-resource",
			mimeType: "text/plain",
			op: func(req *mcp.ReadResourceRequest, w io.Writer) error {
				return errNotFound
			},
			uri:     "test://missing",
			wantErr: errNotFound,
		},
		{
			name:     "empty output",
			resName:  "empty-resource",
			mimeType: "text/plain",
			op: func(req *mcp.ReadResourceRequest, w io.Writer) error {
				return nil
			},
			uri:      "test://empty",
			wantText: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := GenResourceHandler(tt.resName, tt.mimeType, tt.op)
			req := &mcp.ReadResourceRequest{
				Params: &mcp.ReadResourceParams{URI: tt.uri},
			}

			result, err := handler(context.Background(), req)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
				if result != nil {
					t.Errorf("expected nil result on error, got %v", result)
				}
				return
			}

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
			if c.URI != tt.uri {
				t.Errorf("URI = %q, want %q", c.URI, tt.uri)
			}
			if c.MIMEType != tt.mimeType {
				t.Errorf("MIMEType = %q, want %q", c.MIMEType, tt.mimeType)
			}
			if c.Text != tt.wantText {
				t.Errorf("Text = %q, want %q", c.Text, tt.wantText)
			}
		})
	}
}
