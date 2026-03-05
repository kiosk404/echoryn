package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/kiosk404/echoryn/pkg/skills"
	ejson "github.com/kiosk404/echoryn/pkg/utils/json"
)

// ViewSkillTool allows agents to load full skill content on demand.
type ViewSkillTool struct {
	registry *skills.Registry
}

// ViewSkillArgs defines the arguments for the view_skill tool.
type ViewSkillArgs struct {
	// Name is the skill name to view.
	Name string `json:"name"`
	// Section optionally specifies a specific section to extract.
	Section string `json:"section,omitempty"`
	// TOC when true, returns only the table of contents (all headings).
	TOC bool `json:"toc,omitempty"`
}

// NewViewSkillTool creates a new view_skill tool.
func NewViewSkillTool(registry *skills.Registry) *ViewSkillTool {
	return &ViewSkillTool{registry: registry}
}

// Info returns the tool's schema information.
func (t *ViewSkillTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "view_skill",
		Desc: `View the full content of a skill's instructions. Use this tool when:
- A task matches an available skill from <available_skills>
- You need detailed instructions for a specific workflow
- The skill description indicates it's relevant to the current task

Usage patterns:
1. View structure: use toc=true to see all sections (table of contents)
2. View specific section: use section parameter to extract a specific part
3. View full content: use only name parameter`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"name": {
				Type:     schema.String,
				Desc:     "The name of the skill to view (must match a name from <available_skills>)",
				Required: true,
			},
			"section": {
				Type:     schema.String,
				Desc:     "Optional: extract only a specific section by heading (e.g., 'Instructions', 'Examples')",
				Required: false,
			},
			"toc": {
				Type:     schema.Boolean,
				Desc:     "Optional: when true, returns only the table of contents (all headings with indentation)",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun executes the tool and returns the skill content.
func (t *ViewSkillTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args ViewSkillArgs
	if err := ejson.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if args.Name == "" {
		return "", fmt.Errorf("skill name is required")
	}

	if args.TOC && args.Section != "" {
		return "", fmt.Errorf("cannot specify both 'toc' and 'section' parameters")
	}

	content, err := t.registry.GetContent(ctx, args.Name)
	if err != nil {
		return "", fmt.Errorf("failed to load skill '%s': %w", args.Name, err)
	}

	parser := skills.NewParser()

	if args.TOC {
		toc := parser.ExtractTOC(content)
		if toc == "" {
			return "No headings found in this skill.", nil
		}
		return toc, nil
	}

	if args.Section != "" {
		sectionContent := parser.ExtractSection(content, args.Section)
		if sectionContent == "" {
			return "", fmt.Errorf("section '%s' not found in skill '%s'", args.Section, args.Name)
		}
		return sectionContent, nil
	}

	return content, nil
}

// Ensure ViewSkillTool implements tool.InvokableTool.
var _ tool.InvokableTool = (*ViewSkillTool)(nil)
