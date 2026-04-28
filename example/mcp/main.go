package main

import (
	"fmt"
	"log"
	"net/http"

	_ "embed"

	"github.com/aki237/codemode-go/codemode"
	"github.com/aki237/codemode-go/swagtools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed open-meteo.yaml
var openMeteoSpec string

//go:embed rest-countries.yaml
var restCountriesSpec string

func dispatcherFor(baseURL, spec string) (*swagtools.Dispatcher, error) {
	dispatcher := swagtools.NewDispatcher(baseURL, nil)
	return dispatcher, dispatcher.LoadSpec(spec)
}

func main() {
	// Create a server with a single tool.

	conv := codemode.NewConvertor()
	server := mcp.NewServer(&mcp.Implementation{Name: "greeter", Version: "v1.0.0"}, nil)

	omd, err := dispatcherFor("https://api.open-meteo.com", openMeteoSpec)
	if err != nil {
		panic(err)
	}
	rcd, err := dispatcherFor("https://restcountries.com/v3.1", restCountriesSpec)
	if err != nil {
		panic(err)
	}

	for tool, err := range omd.Tools() {
		if err != nil {
			fmt.Printf("WARNING: Skipped tool: %w\n", err)
			continue
		}

		codemode.AddTool(conv, tool, omd.Handler)
	}

	for tool, err := range rcd.Tools() {
		if err != nil {
			fmt.Printf("WARNING: Skipped tool: %w\n", err)
			continue
		}

		codemode.AddTool(conv, tool, rcd.Handler)
	}

	// codemode.AddTool(conv, &mcp.Tool{
	// 	Name:        "call_httpbin",
	// 	Description: "Call sub paths under httpbin.org (like /ip, /anything etc.,)",
	// }, CallHTTPBin)
	conv.Register(server)

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{})
	if err := http.ListenAndServe(":5600", handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
