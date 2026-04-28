package codemode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

type ExecuteInput struct {
	Code string `json:"code" jsonschema:"Starlark code to execute tool calls via call_tool(tool_name, input)"`
}

type ExecuteOutput struct {
	Result any `json:"result" jsonschema:"Result returned by the executed code"`
}

type threadContext struct {
	ctx         context.Context
	callRequest *mcp.CallToolRequest
}

func (c *Convertor) callTool(
	thread *starlark.Thread,
	b *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		toolName string
		input    *starlark.Dict
	)

	tctx, ok := thread.Local(contextKey).(threadContext)
	if !ok {
		return nil, fmt.Errorf("context not found in thread")
	}

	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "tool_name", &toolName, "input", &input); err != nil {
		return nil, err
	}

	inputJSON, err := starlarkDictToJSON(input)
	if err != nil {
		return nil, err
	}

	tool, ok := c.tools[toolName]
	if !ok {
		return nil, fmt.Errorf("tool '%s' not found", toolName)
	}

	argsJSONRaw, err := json.Marshal(inputJSON)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal arguments into json: %w", err)
	}

	_, out, err := tool.call(tctx.ctx, &mcp.CallToolRequest{
		Session: tctx.callRequest.Session,
		Params: &mcp.CallToolParamsRaw{
			Meta:      tctx.callRequest.Params.Meta,
			Name:      toolName,
			Arguments: json.RawMessage(argsJSONRaw),
		},
		Extra: tctx.callRequest.GetExtra(),
	}, inputJSON)
	if err != nil {
		return nil, err
	}

	var outData any
	err = json.Unmarshal(out, &outData)
	if err != nil {
		return nil, err
	}

	sv, err := ToStarlarkValue(outData)
	if err != nil {
		return nil, err
	}

	return sv, nil
}

func starlarkDictToJSON(dict *starlark.Dict) (JSON, error) {
	inputRaw, err := UnpackDict(dict)
	if err != nil {
		return nil, err
	}

	bs, err := json.Marshal(inputRaw)
	if err != nil {
		return nil, err
	}

	return JSON(bs), nil
}

func makeScript(in string, output string) string {
	pad := "  "
	return fmt.Sprintf(`def llm_call():
%s

%s = llm_call()`, pad+strings.ReplaceAll(in, "\n", "\n"+pad), output)
}

const contextKey = "go_context"

func (c *Convertor) ExecuteTool(ctx context.Context, req *mcp.CallToolRequest, input ExecuteInput) (
	*mcp.CallToolResult,
	ExecuteOutput,
	error,
) {
	outputVar := fmt.Sprintf("output_%d", rand.Uint64())
	script := makeScript(input.Code, outputVar)

	// 2. Set up the execution environment (Globals)
	predeclared := starlark.StringDict{
		"call_tool": starlark.NewBuiltin("call_tool", c.callTool),
	}

	// 3. Create a thread to execute the code
	thread := &starlark.Thread{Name: "SandboxThread"}
	thread.SetLocal(contextKey, threadContext{
		ctx:         ctx,
		callRequest: req,
	})

	// 4. Execute the script safely
	// ExecFile takes (thread, filename, source_code, globals)
	globals, err := starlark.ExecFileOptions(&syntax.FileOptions{}, thread, "sandbox.star", script, predeclared)
	if err != nil {
		return nil, ExecuteOutput{}, fmt.Errorf("Execution failed: %v", err)
	}

	// 5. Extract the output variables back into Go
	outputVal, ok := globals[outputVar]
	if !ok {
		return nil, ExecuteOutput{Result: nil}, errors.New("execution unsuccessful")
	}

	data, err := starlarkValueToAny(outputVal)
	if err != nil {
		// TODO: Fix server errors
		return nil, ExecuteOutput{Result: nil}, err
	}

	return nil, ExecuteOutput{Result: data}, nil
}
