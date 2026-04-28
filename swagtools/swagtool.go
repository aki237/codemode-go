package swagtools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ClientFunc returns the HTTP client to use for an operation.
// req is the original incoming MCP call — inspect req.Extra.Header for inbound
// auth tokens or other headers to forward to the upstream service.
type ClientFunc func(ctx context.Context, req *mcp.CallToolRequest) *http.Client

type opInfo struct {
	method string
	path   string
	op     *openapi3.Operation
	params openapi3.Parameters
}

// Dispatcher reads the OpenAPI spec, generates MCP tools for the same.
// The tool handler function also validates the incoming schema spec,
// and calls the upstream endpoint.
type Dispatcher struct {
	loader     *openapi3.Loader
	doc        *openapi3.T
	ops        map[string]opInfo // operationID → opInfo, built on LoadSpec
	clientFunc ClientFunc

	baseURL string
}

// NewDispatcher is used to create a new Dispatcher.
func NewDispatcher(baseURL string, clientFunc ClientFunc) *Dispatcher {
	return &Dispatcher{
		loader: &openapi3.Loader{
			IsExternalRefsAllowed: false,
			IncludeOrigin:         true,
			Context:               context.Background(),
		},
		baseURL:    baseURL,
		clientFunc: clientFunc,
	}
}

func (d *Dispatcher) LoadSpec(specData string) error {
	doc, err := d.loader.LoadFromData([]byte(specData))
	if err != nil {
		return err
	}

	d.doc = doc

	if err := d.doc.Validate(
		context.Background(),
		openapi3.DisableExamplesValidation(),
	); err != nil {
		return err
	}

	d.ops = make(map[string]opInfo)
	for _, path := range d.doc.Paths.InMatchingOrder() {
		pathItem := d.doc.Paths.Find(path)
		for method, op := range pathItem.Operations() {
			if op.OperationID == "" {
				return fmt.Errorf("operation %s %s has no operationId", method, path)
			}
			d.ops[op.OperationID] = opInfo{
				method: method,
				path:   path,
				op:     op,
				params: mergeParams(pathItem.Parameters, op.Parameters),
			}
		}
	}

	return nil
}

func (d *Dispatcher) Tools() iter.Seq2[*mcp.Tool, error] {
	return func(yield func(*mcp.Tool, error) bool) {
		if d.doc == nil {
			return
		}

		secretHeaders := secureHeaderNames(d.doc)

		for _, path := range d.doc.Paths.InMatchingOrder() {
			pathItem := d.doc.Paths.Find(path)
			for method, op := range pathItem.Operations() {
				merged := mergeParams(pathItem.Parameters, op.Parameters)
				tool, err := operationToTool(method, path, op, merged, secretHeaders)
				if !yield(tool, err) {
					return
				}
			}
		}
	}
}

func (d *Dispatcher) Handler(ctx context.Context, req *mcp.CallToolRequest, input any) (
	*mcp.CallToolResult,
	any,
	error,
) {
	info, ok := d.ops[req.Params.Name]
	if !ok {
		return nil, nil, fmt.Errorf("unknown tool %q", req.Params.Name)
	}

	args, _ := input.(map[string]any)
	if args == nil {
		args = map[string]any{}
	}

	httpReq, err := buildRequest(ctx, d.baseURL, info, args)
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}

	client := http.DefaultClient
	if d.clientFunc != nil {
		client = d.clientFunc(ctx, req)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("upstream %d: %s", resp.StatusCode, string(body))
	}

	var result any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &result); err != nil {
			result = map[string]any{"body": string(body)}
		}
	}

	// StructuredContent must be a JSON object; wrap non-object values.
	if _, isMap := result.(map[string]any); !isMap && result != nil {
		result = map[string]any{"result": result}
	}

	return nil, result, nil
}

// buildRequest constructs the outgoing *http.Request from the operation info and tool arguments.
func buildRequest(ctx context.Context, baseURL string, info opInfo, args map[string]any) (*http.Request, error) {
	path := info.path
	queryVals := url.Values{}
	headers := http.Header{}
	var cookies []*http.Cookie
	var bodyBytes []byte

	paramIn := make(map[string]string, len(info.params))
	for _, pRef := range info.params {
		if pRef.Value != nil {
			paramIn[pRef.Value.Name] = pRef.Value.In
		}
	}

	bodyFields := map[string]any{}
	for name, val := range args {
		switch paramIn[name] {
		case "path":
			path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(toStr(val)))
		case "query":
			queryVals.Set(name, toStr(val))
		case "header":
			headers.Set(name, toStr(val))
		case "cookie":
			cookies = append(cookies, &http.Cookie{Name: name, Value: toStr(val)})
		default:
			bodyFields[name] = val
		}
	}

	rawURL := strings.TrimRight(baseURL, "/") + path
	if len(queryVals) > 0 {
		rawURL += "?" + queryVals.Encode()
	}

	var bodyReader io.Reader
	if len(bodyFields) > 0 {
		b, err := json.Marshal(bodyFields)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyBytes = b
		bodyReader = bytes.NewReader(bodyBytes)
	}

	httpReq, err := http.NewRequestWithContext(ctx, strings.ToUpper(info.method), rawURL, bodyReader)
	if err != nil {
		return nil, err
	}

	for k, vals := range headers {
		for _, v := range vals {
			httpReq.Header.Add(k, v)
		}
	}
	for _, c := range cookies {
		httpReq.AddCookie(c)
	}
	if bodyBytes != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	return httpReq, nil
}

// toStr converts a value to its string representation.
// Slices are joined with commas (e.g. for multi-value query params).
func toStr(v any) string {
	switch t := v.(type) {
	case []any:
		parts := make([]string, len(t))
		for i, item := range t {
			parts[i] = fmt.Sprintf("%v", item)
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// secureHeaderNames returns a case-insensitive set of header names that are
// used for authentication and must not be exposed as tool input parameters.
func secureHeaderNames(doc *openapi3.T) map[string]struct{} {
	names := map[string]struct{}{}
	if doc.Components == nil {
		return names
	}

	for _, ref := range doc.Components.SecuritySchemes {
		s := ref.Value
		if s == nil {
			continue
		}
		if s.Type == "apiKey" && strings.EqualFold(s.In, "header") {
			names[strings.ToLower(s.Name)] = struct{}{}
		}
		if s.Type == "http" {
			names["authorization"] = struct{}{}
		}
	}

	return names
}

// mergeParams merges path-level and operation-level parameters; op-level wins on collision.
func mergeParams(pathParams, opParams openapi3.Parameters) openapi3.Parameters {
	seen := map[string]struct{}{}
	result := make(openapi3.Parameters, 0, len(opParams)+len(pathParams))
	for _, ref := range opParams {
		if ref.Value == nil {
			continue
		}
		key := ref.Value.In + ":" + ref.Value.Name
		seen[key] = struct{}{}
		result = append(result, ref)
	}
	for _, ref := range pathParams {
		if ref.Value == nil {
			continue
		}
		key := ref.Value.In + ":" + ref.Value.Name
		if _, ok := seen[key]; !ok {
			result = append(result, ref)
		}
	}
	return result
}

func operationToTool(method, path string, op *openapi3.Operation, params openapi3.Parameters, secretHeaders map[string]struct{}) (*mcp.Tool, error) {
	if op.OperationID == "" {
		return nil, fmt.Errorf("operation %s %s has no operationId", method, path)
	}

	desc := strings.TrimSpace(op.Summary + "\n" + op.Description)

	inputSchema, err := buildInputSchema(params, op.RequestBody, secretHeaders)
	if err != nil {
		return nil, fmt.Errorf("operation %s: %w", op.OperationID, err)
	}

	return &mcp.Tool{
		Name:        op.OperationID,
		Description: desc,
		InputSchema: inputSchema,
	}, nil
}

func buildInputSchema(params openapi3.Parameters, requestBody *openapi3.RequestBodyRef, secretHeaders map[string]struct{}) (json.RawMessage, error) {
	properties := map[string]json.RawMessage{}
	var required []string

	for _, pRef := range params {
		p := pRef.Value
		if p == nil {
			continue
		}
		// skip secret auth headers
		if strings.EqualFold(p.In, "header") {
			if _, secret := secretHeaders[strings.ToLower(p.Name)]; secret {
				continue
			}
		}

		var propSchema json.RawMessage
		if p.Schema == nil || p.Schema.Value == nil {
			propSchema = json.RawMessage(`{}`)
		} else {
			m := derefSchema(p.Schema)
			if p.Description != "" {
				m["description"] = p.Description
			}
			b, err := json.Marshal(m)
			if err != nil {
				return nil, fmt.Errorf("param %s: %w", p.Name, err)
			}
			propSchema = b
		}
		properties[p.Name] = propSchema
		if p.Required {
			required = append(required, p.Name)
		}
	}

	if requestBody != nil && requestBody.Value != nil {
		rb := requestBody.Value
		mt := rb.Content.Get("application/json")
		if mt != nil && mt.Schema != nil && mt.Schema.Value != nil {
			bodySchema := mt.Schema.Value
			bodyRequired := map[string]struct{}{}
			for _, r := range bodySchema.Required {
				bodyRequired[r] = struct{}{}
			}
			for propName, propRef := range bodySchema.Properties {
				if propRef.Value == nil {
					properties[propName] = json.RawMessage(`{}`)
				} else {
					b, err := json.Marshal(derefSchema(propRef))
					if err != nil {
						return nil, fmt.Errorf("body property %s: %w", propName, err)
					}
					properties[propName] = b
				}
				if _, req := bodyRequired[propName]; req && rb.Required {
					required = append(required, propName)
				}
			}
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	return json.Marshal(schema)
}

// derefSchema converts an OpenAPI SchemaRef into a plain map[string]any JSON schema,
// always following .Value instead of emitting $ref, so callers get a fully inlined schema.
func derefSchema(ref *openapi3.SchemaRef) map[string]any {
	if ref == nil || ref.Value == nil {
		return map[string]any{}
	}
	s := ref.Value
	m := map[string]any{}

	if s.Description != "" {
		m["description"] = s.Description
	}
	if len(s.Type.Slice()) == 1 {
		m["type"] = s.Type.Slice()[0]
	} else if len(s.Type.Slice()) > 1 {
		m["type"] = s.Type.Slice()
	}
	if s.Format != "" {
		m["format"] = s.Format
	}
	if len(s.Enum) > 0 {
		m["enum"] = s.Enum
	}
	if s.Default != nil {
		m["default"] = s.Default
	}
	if s.Example != nil {
		m["example"] = s.Example
	}
	if s.Min != nil {
		m["minimum"] = *s.Min
	}
	if s.Max != nil {
		m["maximum"] = *s.Max
	}
	if s.MinLength > 0 {
		m["minLength"] = s.MinLength
	}
	if s.MaxLength != nil {
		m["maxLength"] = *s.MaxLength
	}
	if s.Pattern != "" {
		m["pattern"] = s.Pattern
	}
	if s.Nullable {
		m["nullable"] = true
	}

	if s.Items != nil {
		m["items"] = derefSchema(s.Items)
	}

	if len(s.Properties) > 0 {
		props := make(map[string]any, len(s.Properties))
		for k, v := range s.Properties {
			props[k] = derefSchema(v)
		}
		m["properties"] = props
	}
	if len(s.Required) > 0 {
		m["required"] = s.Required
	}

	if len(s.AllOf) > 0 {
		sub := make([]any, len(s.AllOf))
		for i, v := range s.AllOf {
			sub[i] = derefSchema(v)
		}
		m["allOf"] = sub
	}
	if len(s.AnyOf) > 0 {
		sub := make([]any, len(s.AnyOf))
		for i, v := range s.AnyOf {
			sub[i] = derefSchema(v)
		}
		m["anyOf"] = sub
	}
	if len(s.OneOf) > 0 {
		sub := make([]any, len(s.OneOf))
		for i, v := range s.OneOf {
			sub[i] = derefSchema(v)
		}
		m["oneOf"] = sub
	}

	return m
}
