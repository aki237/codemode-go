# Examples

[![Run on Replit](https://replit.com/badge/github/aki237/codemode-go)](https://replit.com/new/go?url=https://github.com/aki237/codemode-go)

## Using codemode only

Register tools from any source and use codemode's search, schema, and execution tools:

```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/aki237/codemode-go/codemode"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	conv := codemode.NewConvertor()
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "tool-executor",
		Version: "v1.0.0",
	}, nil)

	// Register your custom tools
	codemode.AddTool(conv, &mcp.Tool{
		Name:        "greet",
		Description: "Greet a person",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct {
		Name string `json:"name"`
	}) (*mcp.CallToolResult, string, error) {
		return nil, "Hello, " + input.Name, nil
	})

	conv.Register(server)

	handler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{},
	)
	log.Fatal(http.ListenAndServe(":5600", handler))
}
```

## Using swagtools only

Convert OpenAPI specs to MCP tools without codemode:

```go
package main

import (
	"log"
	"net/http"

	"github.com/aki237/codemode-go/swagtools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "api-tools",
		Version: "v1.0.0",
	}, nil)

	dispatcher := swagtools.NewDispatcher("https://api.example.com", nil)
	if err := dispatcher.LoadSpec(openAPISpecYAML); err != nil {
		log.Fatal(err)
	}

	for tool, err := range dispatcher.Tools() {
		if err != nil {
			log.Printf("warning: %v", err)
			continue
		}
		mcp.AddTool(server, tool, dispatcher.Handler)
	}

	handler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{},
	)
	log.Fatal(http.ListenAndServe(":5600", handler))
}
```

## Using both codemode and swagtools

Register OpenAPI tools with codemode's search, schema, and execution capabilities:

```go
package main

import (
	"log"
	"net/http"

	"github.com/aki237/codemode-go/codemode"
	"github.com/aki237/codemode-go/swagtools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	conv := codemode.NewConvertor()
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "api-executor",
		Version: "v1.0.0",
	}, nil)

	dispatcher := swagtools.NewDispatcher("https://api.example.com", nil)
	if err := dispatcher.LoadSpec(openAPISpecYAML); err != nil {
		log.Fatal(err)
	}

	for tool, err := range dispatcher.Tools() {
		if err != nil {
			log.Printf("warning: %v", err)
			continue
		}
		codemode.AddTool(conv, tool, dispatcher.Handler)
	}

	conv.Register(server)

	handler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{},
	)
	log.Fatal(http.ListenAndServe(":5600", handler))
}
```
