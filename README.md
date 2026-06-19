# cobra-mcp

Turn any [Cobra](https://github.com/spf13/cobra) CLI into an [MCP](https://modelcontextprotocol.io/) server.

`cobra-mcp` gives you a pre-configured `*mcp.Server` and a `mcp` subcommand (stdio + HTTP) in one call.
You keep full control over tool/resource/prompt schemas — the package handles wiring, transport, and logging.

## Install

```
go get github.com/eat-pray-ai/cobra-mcp
```

## Quick Start

See [`examples/main.go`](examples/main.go) for a complete working example with tools, resources, and prompts.

```go
server, mcpCmd := cobramcp.ServerAndCommand(&cobramcp.Config{
    Name: "myapp", Version: "0.1.0",
})

// Register a tool
mcp.AddTool(server, &mcp.Tool{...}, cobramcp.GenToolHandler("hello", hello))

// Register a resource
server.AddResource(&mcp.Resource{...}, cobramcp.GenResourceHandler("version", "application/json", version))

// Register a prompt
server.AddPrompt(&mcp.Prompt{...}, cobramcp.GenPromptHandler("review", review))

rootCmd.AddCommand(mcpCmd)
```

```shell
# Use as MCP server (stdio)
myapp mcp

# Use as MCP server (HTTP)
myapp mcp --mode http --port 8080

# Use as MCP server (HTTP with OAuth)
myapp mcp --mode http --port 8080 --baseUrl https://mcp.example.com
```

## API

| Function | Signature | Purpose |
|----------|-----------|---------|
| `ServerAndCommand` | `(cfg *Config) (*mcp.Server, *cobra.Command)` | Create MCP server + cobra command |
| `GenToolHandler` | `[T any](name string, op func(T, io.Writer) error)` | Typed tool handler with JSON deserialization |
| `GenResourceHandler` | `(name, mimeType string, op func(*mcp.ReadResourceRequest, io.Writer) error)` | Resource handler with MIME type |
| `GenPromptHandler` | `(name string, op func(*mcp.GetPromptRequest) ([]*mcp.PromptMessage, error))` | Multi-message prompt handler |

### Config

| Field | Default | Description |
|-------|---------|-------------|
| `Name` | — | Server implementation name |
| `Version` | — | Server implementation version |
| `Instructions` | — | Brief server description |
| `PageSize` | `100` | Pagination size |
| `KeepAlive` | `13s` | Keep-alive ping interval |
| `DefaultPort` | `8216` | Default HTTP port |
| `Auth` | `nil` | OAuth config (HTTP only) |
| `ServerOptions` | — | Override full `*mcp.ServerOptions` |

### ContextAware

Tool input types implementing `ContextAware` receive the request context before the operation is invoked:

```go
type ContextAware interface {
    SetContext(context.Context)
}
```

### AuthConfig

When `Auth` is set, the HTTP transport requires OAuth Bearer tokens. See [examples/main.go](examples/main.go) and the `AuthConfig` godoc for details.

## Design

- **Schema-first** — you provide `*jsonschema.Schema` for each tool, no auto-generation
- **In-process** — handlers call your Go functions directly, not via subprocess
- **Transport included** — stdio and HTTP handled by the generated `mcp` command
- **OAuth built-in** — optional MCP OAuth with auto-derived metadata

## License

Apache-2.0