package agentflow

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	pluginPkg "github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// PluginTool adapts a plugin.ToolDefinition to a Enio's tool. tool.InvokableTool interface.
// This bridges the plugin framework's tools system with Eino's ReAct agent,
// following the same pattern as the other built-in tools.
type PluginTool struct {
	def pluginPkg.ToolDefinition
}

var _ tool.InvokableTool = (*PluginTool)(nil)

// Info returns the Eino ToolInfo for this tool, used by the LLM to understand
// the tool's the same pattern as the other built-in tools.
func (p *PluginTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	params := make(map[string]*schema.ParameterInfo, len(p.def.Parameters))

	for _, param := range p.def.Parameters {
		params[param.Name] = &schema.ParameterInfo{
			Desc:     param.Description,
			Type:     toSchemaDataType(param.Type),
			Required: param.Required,
		}
	}

	return &schema.ToolInfo{
		Name:        p.def.Name,
		Desc:        p.def.Description,
		ParamsOneOf: schema.NewParamsOneOfByParams(params),
	}, nil
}

// InvokableRun invokes the plugin tool with the given arguments.
// The argumentsInJSON parameter is expected to be a JSON string that maps parameter names to their values.
func (p *PluginTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var params map[string]interface{}
	if argumentsInJSON != "" && argumentsInJSON != "{}" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &params); err != nil {
			// LLM-generated JSON may contain invalid escape sequences (e.g. shell
			// commands with unescaped backslashes). Attempt to sanitize and retry
			// before giving up.
			sanitized := sanitizeJSON(argumentsInJSON)
			if err = json.Unmarshal([]byte(sanitized), &params); err != nil {
				return "", fmt.Errorf("failed to unmarshal arguments JSON: %w", err)
			}
		}
	}

	if params == nil {
		params = make(map[string]interface{})
	}

	result, err := p.def.Handler(ctx, params)
	if err != nil {
		return "", fmt.Errorf("failed to invoke plugin tool: %w", err)
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal plugin tool result: %w", err)
	}

	return string(resultBytes), nil
}

func (p *PluginTool) IsStream() bool {
	return false
}

// AdaptResult holds the adapted tools and deferred tool names.
type AdaptResult struct {
	// ActiveTools are fully adapted Eino tools (non-deferred).
	ActiveTools []tool.BaseTool
	// DeferredNames are tool names that are deferred (only name sent to LLM).
	DeferredNames []string
}

// AdaptPluginTools converts plugin-registered tools matching the given names to Eino tools.
// Deferred tools (ShouldDefer=true) are NOT converted — only their names are returned
// in DeferredNames. The LLM discovers deferred tools via the tool_search tool.
// If toolNames is empty, all registered tools are adapted.
func AdaptPluginTools(registry *pluginPkg.Registry, toolNames []string) AdaptResult {
	allTools := registry.GetTools()
	var result AdaptResult

	filter := func(def pluginPkg.ToolDefinition) {
		if def.ShouldDefer {
			result.DeferredNames = append(result.DeferredNames, def.Name)
		} else {
			result.ActiveTools = append(result.ActiveTools, &PluginTool{def: def})
		}
	}

	if len(toolNames) == 0 {
		for _, def := range allTools {
			filter(def)
		}
		return result
	}

	nameSet := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		nameSet[name] = struct{}{}
	}
	for name, def := range allTools {
		if _, ok := nameSet[name]; ok {
			filter(def)
		}
	}
	return result
}

// AdaptPluginToolsFromDefs converts a pre-filtered list of ToolDefinitions to Eino tools.
// This is used when the ToolPolicyPipeline has already filtered the definitions,
// so no registry lookup or name matching is needed.
func AdaptPluginToolsFromDefs(defs []pluginPkg.ToolDefinition) AdaptResult {
	var result AdaptResult
	for _, def := range defs {
		if def.ShouldDefer {
			result.DeferredNames = append(result.DeferredNames, def.Name)
		} else {
			result.ActiveTools = append(result.ActiveTools, &PluginTool{def: def})
		}
	}
	return result
}

// invalidEscapeRe matches a backslash followed by a character that is Not a Valid
// JSON escape (\", \\, \/, \b, \f, \n, \r, \t, \uXXXX). Inside JSON strings LLMs
// sometimes produce bare backslashes (e.g. grep patterns like `server\|foo`) which
// violate RFC 8259 and cause parsers like sonic to reject the payload.

var InvalidEscapeRe = regexp.MustCompile(`\\([^"\\/bfnrtu])`)

// sanitizeJSON attempts to fix common LLM-generated JSON issues:
//  1. Invalid escape sequences inside string values (e.g. \| , \' , \> etc.)
//     are converted to valid double-backslash escapes (\\| , \\' , \\>).
//  2. Unescaped control characters (tabs, newlines) inside strings are replaced
//     with their JSON escape equivalents.
func sanitizeJSON(s string) string {
	// Step 1: replace raw control characters that may appear inside string values.
	s = strings.ReplaceAll(s, "\t", `\t`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)

	// Step2:
	s = InvalidEscapeRe.ReplaceAllString(s, `\\$1`)

	return s
}

// toSchemaDataType converts a string type name to the corresponding Eino schema.DataType.
func toSchemaDataType(t string) schema.DataType {
	switch t {
	case "string":
		return schema.String
	case "number":
		return schema.Number
	case "boolean":
		return schema.Boolean
	case "object":
		return schema.Object
	case "array":
		return schema.Array
	default:
		return schema.String
	}
}
