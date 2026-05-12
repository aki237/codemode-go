[![PkgGoDev](https://pkg.go.dev/badge/github.com/aki237/codemode-go)](https://pkg.go.dev/github.com/aki237/codemode-go)

# codemode-go

Convert OpenAPI specifications to MCP (Model Context Protocol) tools with Starlark scripting support.

## Overview

codemode-go provides two modular MCP tools:

 * **codemode** - Code mode MCP wrapper that enables tool searching, schema inspection, and Starlark-based execution for composing multi-step operations.

 * **swagtools** - OpenAPI request executor that converts OpenAPI/Swagger specifications into callable MCP tools, handling parameter validation and HTTP orchestration.

## Installation

```bash
go get github.com/aki237/codemode-go
```

See [EXAMPLES.md](EXAMPLES.md) for usage patterns.

## Core Tools

### search
Search for tools by keyword. Returns results ranked by relevance.

```json
{
  "query": "weather"
}
```

### get_schema
Retrieve detailed parameter and response schemas for tools.

```json
{
  "tools": ["get_weather", "get_forecast"]
}
```

### execute
Chain tool calls in Starlark. Use `call_tool(name, params)` to invoke registered tools.

```json
{
  "code": "result = call_tool('get_weather', {'location': 'New York'})\nreturn result['temperature']"
}
```

## Components

- **Convertor**: Central registry for MCP tools with search and schema inspection
- **Dispatcher**: Loads OpenAPI specs and generates MCP tools with HTTP handling
- **Starlark Integration**: Execute multi-step operations through Python-like scripting

## Development

Run tests:
```bash
go test ./...
```

## License

MIT
