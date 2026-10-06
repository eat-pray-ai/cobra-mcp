# cobra-mcp

[![GitHub License](https://img.shields.io/github/license/eat-pray-ai/cobra-mcp?style=flat-square)](https://github.com/eat-pray-ai/cobra-mcp?tab=Apache-2.0-1-ov-file)
[![Go Reference](https://pkg.go.dev/badge/github.com/eat-pray-ai/cobra-mcp?style=flat-square)](https://pkg.go.dev/github.com/eat-pray-ai/cobra-mcp)
[![Go Coverage](https://github.com/eat-pray-ai/cobra-mcp/wiki/coverage.svg)](https://raw.githack.com/wiki/eat-pray-ai/cobra-mcp/coverage.html)
[![GitHub Actions test Status](https://img.shields.io/github/actions/workflow/status/eat-pray-ai/cobra-mcp/test.yml?style=flat-square&logo=githubactions&label=test)](https://github.com/eat-pray-ai/cobra-mcp/actions/workflows/test.yml)
[![GitHub Release](https://img.shields.io/github/v/release/eat-pray-ai/cobra-mcp?sort=semver&style=flat-square&logo=go)](https://github.com/eat-pray-ai/cobra-mcp/releases/latest)

Turn any [Cobra](https://github.com/spf13/cobra) CLI into an [MCP](https://modelcontextprotocol.io/) server.

`cobra-mcp` gives you a pre-configured `*mcp.Server` and a `mcp` subcommand (stdio + HTTP) in one call.
You keep full control over tool/resource/prompt schemas — the package handles wiring and transport.

## Install

```
go get github.com/eat-pray-ai/cobra-mcp
```

## Quick Start

See [`examples/main.go`](examples/main.go) for a complete working example with tools, resources, and prompts.

```go
server, mcpCmd := cobramcp.ServerAndCommand(&cobramcp.Config{
    Name: "myapp", Version: "0.1.0",
    ServerOptions: &mcp.ServerOptions{Instructions: "A demo CLI with MCP support"},
})

// Register a tool
mcp.AddTool(server, &mcp.Tool{...}, cobramcp.GenToolHandler("hello", hello))

// Optional: observe or block generated tool executions
cobramcp.SetDefaultToolExecutionHook(myHook)

// Register a resource
server.AddResource(&mcp.Resource{...}, cobramcp.GenResourceHandler("version", "application/json", version))

// Register a prompt
server.AddPrompt(&mcp.Prompt{...}, cobramcp.GenPromptHandler("review", review))

rootCmd.AddCommand(mcpCmd)
```

```shell
# Use as MCP server (stdio)
myapp mcp

# Use as MCP server (HTTP, stateless by default for MCP 2026-07-28)
myapp mcp --mode http --port 8080

# Use as MCP server (HTTP, bind all interfaces)
myapp mcp --mode http --port 8080 --host 0.0.0.0

# Use as MCP server (HTTP, stateful sessions for legacy clients)
myapp mcp --mode http --port 8080 --stateless=false

# Use as MCP server (HTTP with OAuth)
myapp mcp --mode http --port 8080 --baseUrl https://mcp.example.com
```

## API

| Function                        | Signature                                                                                                  | Purpose                                               |
|---------------------------------|------------------------------------------------------------------------------------------------------------|-------------------------------------------------------|
| `ServerAndCommand`              | `(cfg *Config) (*mcp.Server, *cobra.Command)`                                                              | Create MCP server + cobra command                     |
| `GenToolHandler`                | `[T any](name string, op func(T, io.Writer) error)`                                                        | Typed tool handler with JSON deserialization          |
| `GenToolHandlerWithMRTR`        | `[T any](name string, op func(context.Context, *mcp.CallToolRequest, T, io.Writer) (*mcp.CallToolResult, error))` | Typed tool handler with multi round-trip support      |
| `SetDefaultToolExecutionHook`   | `(hook ToolExecutionHook)`                                                                                 | Install or clear a process-wide generated-tool hook   |
| `GenResourceHandler`            | `(name, mimeType string, op func(context.Context, *mcp.ReadResourceRequest, io.Writer) error)`             | Resource handler with MIME type                       |
| `GenPromptHandler`              | `(name string, op func(*mcp.GetPromptRequest) ([]*mcp.PromptMessage, error))`                              | Multi-message prompt handler                          |

### Config

| Field           | Default | Description                                                    |
|-----------------|---------|----------------------------------------------------------------|
| `Name`          | —       | Server implementation name                                     |
| `Version`       | —       | Server implementation version                                  |
| `Auth`          | `nil`   | OAuth config (HTTP only)                                       |
| `HTTPOptions`   | `nil`   | Override `*mcp.StreamableHTTPOptions` (HTTP only)              |
| `ListCacheTTL`  | `0`     | TTL hint (ms) on `*/list` responses; 0 = no caching            |
| `ServerOptions` | `nil`   | `*mcp.ServerOptions` (Instructions, PageSize, KeepAlive, etc.) |

`PageSize` defaults to 100 and `KeepAlive` defaults to 13s unless `HTTPOptions.Stateless` is true (no session to ping).

### ContextAware

Tool input types implementing `ContextAware` receive the request context before the operation is invoked:

```go
type ContextAware interface {
    SetContext(context.Context)
}
```

### ToolExecutionHook

Generated tool handlers can be observed or blocked with a process-wide `ToolExecutionHook`:

```go
type ToolExecutionHook interface {
    BeforeTool(ctx context.Context, toolName string, input any) error
    AfterTool(ctx context.Context, toolName string, input any, err error)
}
```

Install or clear the hook with:

```go
cobramcp.SetDefaultToolExecutionHook(myHook)
cobramcp.SetDefaultToolExecutionHook(nil) // clear
```

`BeforeTool` runs after `ContextAware` injection and before the tool operation. If it returns an error, the operation is not called. `AfterTool` runs after the operation returns, including operation error cases. Existing generated handlers read the hook at execution time, so a hook can be installed after tools are registered.

Example:

```go
type auditHook struct{}

func (auditHook) BeforeTool(ctx context.Context, toolName string, input any) error {
    slog.InfoContext(ctx, "tool starting", "tool", toolName, "input", input)
    return nil
}

func (auditHook) AfterTool(ctx context.Context, toolName string, input any, err error) {
    slog.InfoContext(ctx, "tool finished", "tool", toolName, "error", err)
}

cobramcp.SetDefaultToolExecutionHook(auditHook{})
```

### AuthConfig

When `Auth` is set, the HTTP transport requires OAuth Bearer tokens. See [examples/main.go](examples/main.go) and the `AuthConfig` godoc for details.

## Design

- **Schema-first** — you provide `*jsonschema.Schema` for each tool, no auto-generation
- **In-process** — handlers call your Go functions directly, not via subprocess
- **Transport included** — stdio and HTTP handled by the generated `mcp` command
- **OAuth built-in** — optional MCP OAuth with auto-derived metadata
- **Hookable tools** — generated tool handlers support centralized audit, policy, and rate-limit hooks

## License

Apache-2.0
