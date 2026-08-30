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
		name string
		run  func(t *testing.T)
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
				result, _, err := handler(
					ctx, &mcp.CallToolRequest{}, contextAwareInput{Name: "test"},
				)
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

				result, _, err := handler(
					context.Background(), &mcp.CallToolRequest{}, plainInput{Name: "world"},
				)
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

				_, _, err := handler(
					context.Background(), &mcp.CallToolRequest{}, plainInput{Name: "x"},
				)
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

func TestGenToolHandlerWithMRTR(t *testing.T) {
	errBoom := errors.New("boom")

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "op returns CallToolResult for MRTR",
			run: func(t *testing.T) {
				handler := GenToolHandlerWithMRTR(
					"test-tool",
					func(ctx context.Context, req *mcp.CallToolRequest, input plainInput, w io.Writer) (*mcp.CallToolResult, error) {
						if req.Params.InputResponses == nil {
							return &mcp.CallToolResult{
								InputRequests: mcp.InputRequestMap{
									"confirm": &mcp.ElicitParams{Message: "proceed?"},
								},
								RequestState: "state-1",
							}, nil
						}
						_, _ = io.WriteString(w, "done")
						return nil, nil
					},
				)

				// First call: no InputResponses → MRTR
				result, _, err := handler(
					context.Background(),
					&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}},
					plainInput{Name: "x"},
				)
				if err != nil {
					t.Fatalf("handler returned error: %v", err)
				}
				if result.InputRequests == nil {
					t.Fatal("expected InputRequests to be set")
				}
				if result.RequestState != "state-1" {
					t.Errorf("RequestState = %q, want %q", result.RequestState, "state-1")
				}

				// Second call: with InputResponses → normal completion
				result, _, err = handler(
					context.Background(),
					&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
						InputResponses: mcp.InputResponseMap{
							"confirm": &mcp.ElicitResult{Action: "accept"},
						},
					}},
					plainInput{Name: "x"},
				)
				if err != nil {
					t.Fatalf("handler returned error: %v", err)
				}
				text := result.Content[0].(*mcp.TextContent).Text
				if text != "done" {
					t.Errorf("text = %q, want %q", text, "done")
				}
			},
		},
		{
			name: "op returns nil result uses buffer",
			run: func(t *testing.T) {
				handler := GenToolHandlerWithMRTR(
					"test-tool",
					func(ctx context.Context, req *mcp.CallToolRequest, input plainInput, w io.Writer) (*mcp.CallToolResult, error) {
						_, _ = io.WriteString(w, "hello "+input.Name)
						return nil, nil
					},
				)

				result, _, err := handler(
					context.Background(),
					&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}},
					plainInput{Name: "world"},
				)
				if err != nil {
					t.Fatalf("handler returned error: %v", err)
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
				handler := GenToolHandlerWithMRTR(
					"test-tool",
					func(ctx context.Context, req *mcp.CallToolRequest, input plainInput, w io.Writer) (*mcp.CallToolResult, error) {
						return nil, errBoom
					},
				)

				_, _, err := handler(
					context.Background(),
					&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}},
					plainInput{Name: "x"},
				)
				if !errors.Is(err, errBoom) {
					t.Errorf("error = %v, want %v", err, errBoom)
				}
			},
		},
		{
			name: "injects context into ContextAware input",
			run: func(t *testing.T) {
				handler := GenToolHandlerWithMRTR(
					"test-tool",
					func(ctx context.Context, req *mcp.CallToolRequest, input contextAwareInput, w io.Writer) (*mcp.CallToolResult, error) {
						val := input.ctx.Value(ctxKey{})
						if val == nil {
							t.Error("expected context value to be set")
							return nil, nil
						}
						if val != "injected" {
							t.Errorf("context value = %v, want 'injected'", val)
						}
						return nil, nil
					},
				)

				ctx := context.WithValue(context.Background(), ctxKey{}, "injected")
				result, _, err := handler(
					ctx,
					&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}},
					contextAwareInput{Name: "test"},
				)
				if err != nil {
					t.Fatalf("handler returned error: %v", err)
				}
				if result == nil {
					t.Fatal("expected non-nil result")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestConfirmThen(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "first call returns elicitation request",
			run: func(t *testing.T) {
				op := ConfirmThen(
					func(input plainInput) string { return "Delete " + input.Name + "?" },
					func(input plainInput, w io.Writer) error {
						_, _ = io.WriteString(w, "deleted")
						return nil
					},
				)
				handler := GenToolHandlerWithMRTR("test-tool", op)

				result, _, err := handler(
					context.Background(),
					&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}},
					plainInput{Name: "foo"},
				)
				if err != nil {
					t.Fatalf("handler returned error: %v", err)
				}
				if result.InputRequests == nil {
					t.Fatal("expected InputRequests to be set")
				}
				ep, ok := result.InputRequests["confirm"].(*mcp.ElicitParams)
				if !ok {
					t.Fatalf("InputRequests[confirm] type = %T, want *ElicitParams", result.InputRequests["confirm"])
				}
				if ep.Message != "Delete foo?" {
					t.Errorf("message = %q, want %q", ep.Message, "Delete foo?")
				}
				if result.RequestState != "awaiting_confirm" {
					t.Errorf("RequestState = %q, want %q", result.RequestState, "awaiting_confirm")
				}
			},
		},
		{
			name: "accept executes operation",
			run: func(t *testing.T) {
				op := ConfirmThen(
					func(input plainInput) string { return "Confirm?" },
					func(input plainInput, w io.Writer) error {
						_, _ = io.WriteString(w, "deleted "+input.Name)
						return nil
					},
				)
				handler := GenToolHandlerWithMRTR("test-tool", op)

				result, _, err := handler(
					context.Background(),
					&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
						InputResponses: mcp.InputResponseMap{
							"confirm": &mcp.ElicitResult{Action: "accept"},
						},
						RequestState: "awaiting_confirm",
					}},
					plainInput{Name: "bar"},
				)
				if err != nil {
					t.Fatalf("handler returned error: %v", err)
				}
				text := result.Content[0].(*mcp.TextContent).Text
				if text != "deleted bar" {
					t.Errorf("text = %q, want %q", text, "deleted bar")
				}
			},
		},
		{
			name: "decline returns cancellation",
			run: func(t *testing.T) {
				op := ConfirmThen(
					func(input plainInput) string { return "Confirm?" },
					func(input plainInput, w io.Writer) error {
						t.Error("op should not be called on decline")
						return nil
					},
				)
				handler := GenToolHandlerWithMRTR("test-tool", op)

				result, _, err := handler(
					context.Background(),
					&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
						InputResponses: mcp.InputResponseMap{
							"confirm": &mcp.ElicitResult{Action: "decline"},
						},
					}},
					plainInput{Name: "baz"},
				)
				if err != nil {
					t.Fatalf("handler returned error: %v", err)
				}
				if !result.IsError {
					t.Error("expected IsError to be true")
				}
				text := result.Content[0].(*mcp.TextContent).Text
				if text != "canceled by user" {
					t.Errorf("text = %q, want %q", text, "canceled by user")
				}
			},
		},
		{
			name: "op error is propagated",
			run: func(t *testing.T) {
				errFail := errors.New("delete failed")
				op := ConfirmThen(
					func(input plainInput) string { return "Confirm?" },
					func(input plainInput, w io.Writer) error {
						return errFail
					},
				)
				handler := GenToolHandlerWithMRTR("test-tool", op)

				_, _, err := handler(
					context.Background(),
					&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
						InputResponses: mcp.InputResponseMap{
							"confirm": &mcp.ElicitResult{Action: "accept"},
						},
					}},
					plainInput{Name: "x"},
				)
				if !errors.Is(err, errFail) {
					t.Errorf("error = %v, want %v", err, errFail)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestGenPromptHandler(t *testing.T) {
	errMissing := errors.New("missing required argument: name")

	tests := []struct {
		name         string
		promptName   string
		op           func(*mcp.GetPromptRequest) ([]*mcp.PromptMessage, error)
		args         map[string]string
		wantErr      error
		wantMessages []*mcp.PromptMessage
	}{
		{
			name:       "success with arguments",
			promptName: "greet",
			op: func(req *mcp.GetPromptRequest) ([]*mcp.PromptMessage, error) {
				return []*mcp.PromptMessage{
					{
						Role:    "user",
						Content: &mcp.TextContent{Text: "Say hi to " + req.Params.Arguments["name"]},
					},
				}, nil
			},
			args: map[string]string{"name": "Pat"},
			wantMessages: []*mcp.PromptMessage{
				{
					Role:    "user",
					Content: &mcp.TextContent{Text: "Say hi to Pat"},
				},
			},
		},
		{
			name:       "multi-message conversation",
			promptName: "review",
			op: func(req *mcp.GetPromptRequest) ([]*mcp.PromptMessage, error) {
				return []*mcp.PromptMessage{
					{
						Role:    "user",
						Content: &mcp.TextContent{Text: "Review this code: " + req.Params.Arguments["code"]},
					},
					{
						Role:    "assistant",
						Content: &mcp.TextContent{Text: "I'll review the code for correctness and style."},
					},
				}, nil
			},
			args: map[string]string{"code": "fmt.Println()"},
			wantMessages: []*mcp.PromptMessage{
				{
					Role:    "user",
					Content: &mcp.TextContent{Text: "Review this code: fmt.Println()"},
				},
				{
					Role:    "assistant",
					Content: &mcp.TextContent{Text: "I'll review the code for correctness and style."},
				},
			},
		},
		{
			name:       "op returns error",
			promptName: "greet",
			op: func(req *mcp.GetPromptRequest) ([]*mcp.PromptMessage, error) {
				return nil, errMissing
			},
			args:    map[string]string{},
			wantErr: errMissing,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				handler := GenPromptHandler(tt.promptName, tt.op)
				req := &mcp.GetPromptRequest{
					Params: &mcp.GetPromptParams{
						Name:      tt.promptName,
						Arguments: tt.args,
					},
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
				if len(result.Messages) != len(tt.wantMessages) {
					t.Fatalf(
						"got %d messages, want %d",
						len(result.Messages), len(tt.wantMessages),
					)
				}
				for i, msg := range result.Messages {
					want := tt.wantMessages[i]
					if msg.Role != want.Role {
						t.Errorf("message[%d] Role = %q, want %q", i, msg.Role, want.Role)
					}
					gotText := msg.Content.(*mcp.TextContent).Text
					wantText := want.Content.(*mcp.TextContent).Text
					if gotText != wantText {
						t.Errorf("message[%d] Text = %q, want %q", i, gotText, wantText)
					}
				}
			},
		)
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
		t.Run(
			tt.name, func(t *testing.T) {
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
			},
		)
	}
}
