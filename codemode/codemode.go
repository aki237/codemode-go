package codemode

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type JSON = json.RawMessage

type Tool struct {
	// mcpTool contains the underlying mcp.Tool
	// which will be invoked during execute tool call.
	mcpTool *mcp.Tool

	// matchTargets stores the normalized tool metadata fields
	// for query matching during search.
	matchTargets map[string]int

	inputValidator *jsonschema.Resolved
	call           mcp.ToolHandlerFor[JSON, JSON]
}

type Convertor struct {
	tools map[string]Tool
}

func NewConvertor() *Convertor {
	return &Convertor{
		tools: make(map[string]Tool),
	}
}

var (
	pFalse = false
	pTrue  = true
)

func (c *Convertor) Register(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search",
		Title:       "search",
		Description: "Search for available tools by query.\n\nReturns matching tools ranked by relevance.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "search",
			DestructiveHint: &pFalse,
			IdempotentHint:  pTrue,
			OpenWorldHint:   &pFalse,
			ReadOnlyHint:    pTrue,
		},
	}, c.SearchTool)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_schema",
		Title:       "get_schema",
		Description: "Get schema for the passed tools.\n\n Returns detailed schema for the passed tools.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "get_schema",
			DestructiveHint: &pFalse,
			IdempotentHint:  pTrue,
			OpenWorldHint:   &pFalse,
			ReadOnlyHint:    pTrue,
		},
	}, c.GetSchemaTool)
	mcp.AddTool(srv, &mcp.Tool{
		Name:  "execute",
		Title: "execute",
		Description: `Chain "call_tool(...)" calls in one Starlark (Python-ish) block; prefer returning the final answer from a single block.
Use "return" to produce output.
Only "call_tool(tool_name: str, params: dict) -> Any" is available in scope.`,
		Annotations: &mcp.ToolAnnotations{
			Title:          "execute",
			IdempotentHint: pFalse,
			OpenWorldHint:  &pFalse,
			ReadOnlyHint:   pFalse,
		},
	}, c.ExecuteTool)
}

func AddTool[In, Out any](c *Convertor, t *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	matches := map[string]int{
		normalizeAndStem(t.Name):        6,
		normalizeAndStem(t.Title):       5,
		normalizeAndStem(t.Description): 4,
	}
	if t.Annotations != nil && t.Annotations.Title != "" {
		matches[normalizeAndStem(t.Annotations.Title)] = 2
	}

	tx := Tool{
		mcpTool:      t,
		matchTargets: matches,
		call: func(ctx context.Context, ctr *mcp.CallToolRequest, input JSON) (*mcp.CallToolResult, JSON, error) {
			in := new(In)
			err := json.Unmarshal(input, in)
			if err != nil {
				return nil, nil, err
			}

			res, out, err := handler(ctx, ctr, *in)
			if err != nil {
				return nil, nil, err
			}

			bs, err := json.Marshal(out)
			if err != nil {
				return nil, nil, err
			}

			return res, JSON(bs), nil
		},
	}

	setSchema[In, Out](&tx)

	c.tools[t.Name] = tx
}

func setSchema[In, Out any](tx *Tool) {
	if tx.mcpTool.InputSchema == nil {
		// Special handling for an "any" input: treat as an empty object.
		if reflect.TypeFor[In]() == reflect.TypeFor[any]() {
			tx.mcpTool.InputSchema = &jsonschema.Schema{Type: "object"}
		} else {
			schema, err := jsonschema.For[In](&jsonschema.ForOptions{IgnoreInvalidTypes: true})
			if err == nil {
				tx.mcpTool.InputSchema = schema
				schema.MarshalJSON()

				resolved, err := schema.Resolve(&jsonschema.ResolveOptions{})
				if err == nil {
					tx.inputValidator = resolved
				}
			}
		}
	}

	if tx.mcpTool.OutputSchema == nil {
		if reflect.TypeFor[Out]() == reflect.TypeFor[any]() {
			tx.mcpTool.OutputSchema = &jsonschema.Schema{Type: "object"}
		} else {

			schema, err := jsonschema.For[Out](&jsonschema.ForOptions{IgnoreInvalidTypes: true})
			if err == nil {
				tx.mcpTool.OutputSchema = schema
			}
		}
	}
}
