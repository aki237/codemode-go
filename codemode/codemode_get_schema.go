package codemode

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetSchemaInput struct {
	Tools []string `json:"tools" jsonschema:"List of tool names for which details schema is needed"`
}

type GetSchemaOutput struct {
	Results []ToolDetail
}

func (c *Convertor) GetSchemaTool(ctx context.Context, req *mcp.CallToolRequest, input GetSchemaInput) (
	*mcp.CallToolResult,
	GetSchemaOutput,
	error,
) {
	gso := GetSchemaOutput{
		Results: make([]ToolDetail, 0, len(input.Tools)),
	}

	for _, k := range input.Tools {
		tool, ok := c.tools[k]
		if !ok {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: fmt.Sprintf("tool '%s' not found", k),
					},
				},
				IsError: true,
			}, gso, nil
		}

		gso.Results = append(gso.Results, ToolDetail{
			Name:         tool.mcpTool.Name,
			Description:  tool.mcpTool.Description,
			InputSchema:  tool.mcpTool.InputSchema,
			OutputSchema: tool.mcpTool.OutputSchema,
		})
	}

	return nil, gso, nil
}
