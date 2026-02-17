package agentflow

import (
	"context"
	"fmt"

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
			return "", fmt.Errorf("failed to unmarshal arguments JSON: %w", err)
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

// AdaptPluginTools converts plugin-registered tools matching the given names to Eino tools.
// If no names are provided, all registered tools are converted.
// If toolNames is empty, all registered tools are adapted.
func AdaptPluginTools(registry *pluginPkg.Registry, toolNames []string) []tool.BaseTool {
	allTools := registry.GetTools()
	tools := make([]tool.BaseTool, 0, len(toolNames))

	if len(toolNames) == 0 {
		for _, def := range allTools {
			tools = append(tools, &PluginTool{def: def})
		}
		return tools
	}

	nameSet := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		nameSet[name] = struct{}{}
	}

	for name, def := range allTools {
		if _, ok := nameSet[name]; ok {
			tools = append(tools, &PluginTool{def: def})
		}
	}

	return tools
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
