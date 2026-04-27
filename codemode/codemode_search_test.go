package codemode

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func dummy[In, Out any](_ context.Context, request *mcp.CallToolRequest, input In) (result *mcp.CallToolResult, output Out, _ error) {
	return nil, *new(Out), nil
}

type Empty struct{}

type ID struct {
	ID string `json:"id"`
}

type ControlPlane struct {
	ID
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

type Plugin struct {
	ID
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	PluginConfig map[string]any `json:"plugin_config"`
}

type GetPlugin struct {
	ControlPlaneID string `json:"control_plane_id"`
	ID             string `json:"id"`
}

type UpdatePlugin struct {
	GetPlugin
	UpdateInput struct {
		Name         string         `json:"name"`
		Description  string         `json:"description"`
		PluginConfig map[string]any `json:"plugin_config"`
	} `json:"update_input"`
}

type UpdateCP struct {
	ID
	UpdateInput struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"update_input"`
}

func TestSearch(t *testing.T) {
	c := NewConvertor()

	AddTool(c, &mcp.Tool{
		Name:        "list_control_planes",
		Title:       "List Control Planes",
		Description: "List all the control planes configured in Konnect",
	}, dummy[Empty, []ControlPlane])
	AddTool(c, &mcp.Tool{
		Name:        "get_control_plane",
		Title:       "Get Control Plane",
		Description: "Get details of a control plane",
	}, dummy[string, ControlPlane])
	AddTool(c, &mcp.Tool{
		Name:        "update_control_plane",
		Title:       "Update Control Plane",
		Description: "Update details of a control plane",
	}, dummy[UpdateCP, ControlPlane])
	AddTool(c, &mcp.Tool{
		Name:        "delete_control_plane",
		Title:       "Delete Control Plane",
		Description: "Delete a control plane",
	}, dummy[string, Empty])
	AddTool(c, &mcp.Tool{
		Name:        "list_plugins",
		Title:       "List Plugins",
		Description: "List all the configured plugins in a control plane",
	}, dummy[string, []Plugin])
	AddTool(c, &mcp.Tool{
		Name:        "get_plugin",
		Title:       "Get Plugins",
		Description: "Get details of a configured plugin",
	}, dummy[GetPlugin, Plugin])
	AddTool(c, &mcp.Tool{
		Name:        "upsert_plugin",
		Title:       "Upsert Plugins",
		Description: "Update (or) Upsert plugin configuration in a control plane",
	}, dummy[UpdatePlugin, Plugin])
	AddTool(c, &mcp.Tool{
		Name:        "reorder_plugins",
		Title:       "Reorder Plugins",
		Description: "Reorder is used to reorder the execution order of plugins in a control plane",
	}, dummy[[]string, Plugin])

	limit := 1
	mcpOut, sout, err := c.SearchTool(t.Context(), nil, SearchInput{
		Query:  "update plugin",
		Detail: "detail",
		Limit:  &limit,
	})
	if err != nil {
		t.Fatal(err)
	}

	if mcpOut != nil {
		for _, v := range mcpOut.Content {
			t.Logf("Content: %+v", v.(*mcp.TextContent).Text)
		}
	}

	if sout.Hits != nil {
		bs, _ := json.MarshalIndent(sout.Hits, "", "  ")
		t.Logf("Structured Out: %+v\n", string(bs))
	}

	// for name, tool := range c.tools {
	// 	bs, _ := json.MarshalIndent(tool.matchTargets, "", "  ")
	// 	t.Logf("%s:\n%s\n", name, string(bs))
	// }

}
