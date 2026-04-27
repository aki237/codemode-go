package codemode

import (
	"bytes"
	"context"
	"sort"
	"strings"
	"text/template"
	"unicode"

	_ "embed"

	"github.com/kljensen/snowball"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	//go:embed templates/brief.gotmpl
	briefTemplate string

	//go:embed templates/detail.gotmpl
	detailTemplate string
)

type SearchInput struct {
	Query  string `json:"query" jsonschema:"Search terms (space separated) to find availabel tools"`
	Detail string `json:"detail" jsonschema:"Level of detail the search output is required in. (brief, detail, full)"`
	Limit  *int   `json:"limit" jsonschema:"Maximum number of results to return"`
	// ScoreThreshold int    `json:"score_threshold" jsonschema:"Minimum score threshold"`
}

type ToolDetail struct {
	Name         string `json:"name" jsonschema:"Name of the tool"`
	Description  string `json:"description" jsonschema:"Description of the tool"`
	InputSchema  any    `json:"input_schema" jsonschema:"Input parameters of the tool"`
	OutputSchema any    `json:"output_schema" jsonschema:"Output structure of the tool"`
}

type SearchHit struct {
	ToolDetail
	Score int `json:"score" jsonschema:"Rank score of the result"`
}

type SearchOutput struct {
	Hits []SearchHit `json:"hits" jsonschema:"Results of the search call"`
}

func (c *Convertor) SearchTool(ctx context.Context, req *mcp.CallToolRequest, input SearchInput) (
	*mcp.CallToolResult,
	SearchOutput,
	error,
) {
	limit := uint(10)
	if input.Limit != nil && *input.Limit > 0 {
		limit = uint(*input.Limit)
	}
	hits := c.search(input.Query, uint(limit))

	switch input.Detail {
	case "detail":
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: executeToolsTemplate(detailTemplate, hits),
				},
			},
		}, SearchOutput{}, nil
	case "full":
		return nil, SearchOutput{hits}, nil
	default:
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: executeToolsTemplate(briefTemplate, hits),
				},
			},
		}, SearchOutput{}, nil
	}
}

func (c *Convertor) search(query string, count uint) []SearchHit {
	// If count is 0, take the default as 10
	if count == 0 {
		count = 10
	}

	hits := make([]SearchHit, 0, count)

	queryTokens := make([]string, 0)
	for v := range strings.FieldsSeq(query) {
		queryTokens = append(queryTokens, normalizeAndStem(v))
	}

	exactPhrase := " " + strings.Join(queryTokens, " ") + " "

	for toolName, tool := range c.tools {
		score := 0

		for text, bonus := range tool.matchTargets {
			score += scoreField(queryTokens, exactPhrase, text, bonus, 15)
		}

		if score > 0 {
			hits = append(hits, SearchHit{
				ToolDetail: ToolDetail{
					Name:         toolName,
					Description:  tool.mcpTool.Description,
					InputSchema:  tool.mcpTool.InputSchema,
					OutputSchema: tool.mcpTool.OutputSchema,
				},
				Score: score,
			})
		}
	}

	sort.Slice(hits, func(i int, j int) bool {
		return hits[i].Score > hits[j].Score
	})

	if uint(len(hits)) > count {
		return hits[:count]
	}

	return hits
}

func scoreField(queryTokens []string, exactPhrase string, targetField string, exactBonus, tokenBonus int) int {
	score := 0

	paddedTarget := " " + targetField + " "
	if strings.Contains(paddedTarget, exactPhrase) {
		score += exactBonus
	}

	for _, qToken := range queryTokens {
		paddedToken := " " + qToken + " "

		if strings.Contains(paddedTarget, paddedToken) {
			score += tokenBonus
		}
	}

	return score
}

// normalizeAndStem removes punctuation, converts to lowercase, and stems words
func normalizeAndStem(s string) string {
	// Function to split by non-alphanumeric characters
	f := func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsNumber(c)
	}
	words := strings.FieldsFunc(s, f)

	// Process each word
	for i, word := range words {
		// 1. Lowercase
		lowerWord := strings.ToLower(word)

		// 2. Stem the word (e.g., "listing" -> "list")
		// The third parameter 'true' tells it to treat 'y' as a vowel occasionally (standard)
		stemmedWord, err := snowball.Stem(lowerWord, "english", true)
		if err == nil {
			words[i] = stemmedWord
		} else {
			words[i] = lowerWord // Fallback if stemming fails
		}
	}

	// Join back into a single searchable string
	return strings.Join(words, " ")
}

func executeToolsTemplate(templateData string, tools []SearchHit) string {
	bw := bytes.NewBuffer(nil)

	err := template.Must(template.New("breif.tmpl").Funcs(template.FuncMap{
		"indent": func(spaces int, data string) string {
			pad := strings.Repeat(" ", spaces)
			return pad + strings.ReplaceAll(data, "\n", "\n"+pad)
		},
		"incr": func(num int) int {
			return num + 1
		},
		"incrBy": func(num int, by int) int {
			return num + by
		},
		"sumOf": func(schema any, indent int) struct {
			Schema any
			Indent int
		} {
			return struct {
				Schema any
				Indent int
			}{
				schema, indent,
			}
		},
	}).Parse(templateData)).Execute(bw, map[string]any{
		"Tools": tools,
	})
	if err != nil {
		panic(err)
	}

	return bw.String()
}
