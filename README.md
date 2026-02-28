# cobra-mcp

Turn any [Cobra](https://github.com/spf13/cobra) CLI into an [MCP](https://modelcontextprotocol.io/) server.

`cobra-mcp` gives you a pre-configured `*mcp.Server` and a `mcp` subcommand (stdio + HTTP) in one call.
You keep full control over tool/resource schemas — the package handles wiring, transport, and logging.

## Install

```
go get github.com/eat-pray-ai/cobra-mcp
```

## Quick Start

**`main.go`** — root command, MCP server, and `hello` subcommand wired together:

```go
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

const (
	helloTool   = "hello"
	helloShort  = "Say hello"
	helloLong   = "Say hello to someone by name"
	nameUsage   = "Who to greet"
	defaultName = "World"
)

var name string

var helloSchema = &jsonschema.Schema{
	Type:     "object",
	Required: []string{"name"},
	Properties: map[string]*jsonschema.Schema{
		"name": {
			Type: "string", Description: nameUsage,
			Default: json.RawMessage(`"World"`),
		},
	},
}

type HelloInput struct {
	Name string `json:"name"`
}

func hello(input HelloInput, w io.Writer) error {
	_, err := fmt.Fprintf(w, "Hello, %s!\n", input.Name)
	return err
}

var server, mcpCmd = cobramcp.ServerAndCommand(
	&cobramcp.Config{
		Name:    "myapp",
		Version: "0.1.0",
	},
)

var helloCmd = &cobra.Command{
	Use:   helloTool,
	Short: helloShort,
	Long:  helloLong,
	Run: func(cmd *cobra.Command, args []string) {
		_ = hello(HelloInput{Name: name}, cmd.OutOrStdout())
	},
}

func init() {
	mcp.AddTool(
		server, &mcp.Tool{
			Name: helloTool, Title: helloShort, Description: helloLong,
			InputSchema: helloSchema,
		}, cobramcp.GenToolHandler(helloTool, hello),
	)

	helloCmd.Flags().StringVarP(&name, "name", "n", defaultName, nameUsage)
}

func main() {
	rootCmd := &cobra.Command{Use: "myapp"}
	rootCmd.AddCommand(mcpCmd, helloCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

```

```shell
# Use as CLI
myapp hello --name Gopher
# Hello, Gopher!

# Use as MCP server (stdio, for Claude Desktop / VS Code / Cursor)
myapp mcp

# Use as MCP server (HTTP)
myapp mcp --mode http --port 8080
```

## API

### `ServerAndCommand`

```go
func ServerAndCommand(cfg Config) (*mcp.Server, *cobra.Command)
```

Returns an MCP server and a cobra command that starts it. Register tools and resources on the server, then add the command to your root.

### `Config`

| Field           | Default | Description                                          |
|-----------------|---------|------------------------------------------------------|
| `Name`          | —       | Server implementation name                           |
| `Version`       | —       | Server implementation version                        |
| `Instructions`  | —       | Brief server description for clients                 |
| `PageSize`      | `99`    | Pagination size for list operations                  |
| `KeepAlive`     | `13s`   | Keep-alive ping interval                             |
| `DefaultPort`   | `8216`  | Default port for `--mode http`                       |
| `ServerOptions` | —       | Override full `*mcp.ServerOptions` (ignores above)   |

### `GenToolHandler`

```go
func GenToolHandler[T any](toolName string, op func(T, io.Writer) error) mcp.ToolHandlerFor[T, any]
```

Creates a typed tool handler: deserializes JSON input into `T`, calls `op`, returns output as text content. Logs input/output via MCP session logging.

### `GenResourceHandler`

```go
func GenResourceHandler(name, mimeType string, op func(*mcp.ReadResourceRequest, io.Writer) error) mcp.ResourceHandler
```

Creates a resource handler: calls `op`, returns output with the given MIME type. Logs resource reads via MCP session logging.

## Design

- **Schema-first**: You provide `*jsonschema.Schema` for each tool — no auto-generation, no magic.
- **In-process execution**: Tool handlers call your Go functions directly, not via subprocess.
- **Transport included**: The generated `mcp` command handles stdio and HTTP transports.
- **Minimal API**: Two exported functions (`ServerAndCommand`, `GenToolHandler`) + one config struct.

## License

Apache-2.0
