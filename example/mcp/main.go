package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/aki237/codemode-go/codemode"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Path string `json:"path" jsonschema:"Sub path in httpbin.org to hit."`
}

type Output struct {
	Result any `json:"result" jsonschema:"Data returned from the HTTP request"`
}

func CallHTTPBin(ctx context.Context, req *mcp.CallToolRequest, input Input) (
	*mcp.CallToolResult,
	Output,
	error,
) {
	resp, err := http.Get("https://httpbin.org/" + strings.TrimLeft(input.Path, "/"))
	if err != nil {
		return nil, Output{}, err
	}

	defer resp.Body.Close()

	var j any
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return nil, Output{}, err
	}

	return nil, Output{j}, nil
}

func main() {
	// Create a server with a single tool.

	conv := codemode.NewConvertor()
	server := mcp.NewServer(&mcp.Implementation{Name: "greeter", Version: "v1.0.0"}, nil)

	codemode.AddTool(conv, &mcp.Tool{
		Name:        "call_httpbin",
		Description: "Call sub paths under httpbin.org (like /ip, /anything etc.,)",
	}, CallHTTPBin)
	conv.Register(server)

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{})
	if err := http.ListenAndServe(":5600", handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
