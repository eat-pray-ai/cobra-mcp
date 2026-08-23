package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	cobramcp "github.com/eat-pray-ai/cobra-mcp"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// --- Tool: hello ---

type HelloInput struct {
	Name string `json:"name"`
}

func hello(input HelloInput, w io.Writer) error {
	_, err := fmt.Fprintf(w, "Hello, %s!\n", input.Name)
	return err
}

var helloSchema = &jsonschema.Schema{
	Type:     "object",
	Required: []string{"name"},
	Properties: map[string]*jsonschema.Schema{
		"name": {
			Type: "string", Description: "Who to greet",
			Default: json.RawMessage(`"World"`),
		},
	},
}

// --- Resource: version ---

func version(req *mcp.ReadResourceRequest, w io.Writer) error {
	_, err := fmt.Fprintf(w, `{"version":"0.1.0"}`)
	return err
}

// --- Prompt: review ---

func review(req *mcp.GetPromptRequest) ([]*mcp.PromptMessage, error) {
	code := req.Params.Arguments["code"]
	return []*mcp.PromptMessage{
		{Role: "user", Content: &mcp.TextContent{Text: "Review this code:\n" + code}},
		{Role: "assistant", Content: &mcp.TextContent{Text: "I'll review for correctness and style."}},
	}, nil
}

// --- Wiring ---

var server, mcpCmd = cobramcp.ServerAndCommand(
	&cobramcp.Config{
		Name:         "myapp",
		Version:      "0.1.0",
		Instructions: "A demo CLI with MCP support",
		HTTPOptions: &mcp.StreamableHTTPOptions{
			PropagateRequestCancellation: true,
		},
	},
)

var name string

var helloCmd = &cobra.Command{
	Use:   "hello",
	Short: "Say hello",
	Run: func(cmd *cobra.Command, args []string) {
		_ = hello(HelloInput{Name: name}, cmd.OutOrStdout())
	},
}

func init() {
	helloCmd.Flags().StringVarP(&name, "name", "n", "World", "Who to greet")

	mcp.AddTool(
		server, &mcp.Tool{
			Name: "hello", Title: "Say hello",
			Description: "Say hello to someone by name",
			InputSchema: helloSchema,
		}, cobramcp.GenToolHandler("hello", hello),
	)

	server.AddResource(
		&mcp.Resource{
			URI:      "myapp://version",
			Name:     "version",
			MIMEType: "application/json",
		},
		cobramcp.GenResourceHandler("version", "application/json", version),
	)

	server.AddPrompt(
		&mcp.Prompt{
			Name:        "review",
			Description: "Review code for correctness and style",
			Arguments: []*mcp.PromptArgument{
				{Name: "code", Description: "code to review", Required: true},
			},
		},
		cobramcp.GenPromptHandler("review", review),
	)
}

func main() {
	rootCmd := &cobra.Command{Use: "myapp"}
	rootCmd.AddCommand(mcpCmd, helloCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}