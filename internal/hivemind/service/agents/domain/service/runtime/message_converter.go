package runtime

import (
	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
)

// ToSchemaMessages converts domain messages to Eino schema messages.
func ToSchemaMessages(msgs []*entity.Message) []*schema.Message {
	result := make([]*schema.Message, 0, len(msgs))
	for _, msg := range msgs {
		result = append(result, ToSchemaMessage(msg))
	}
	return result
}

// ToSchemaMessage handles bidirectional message conversion between entity and schema.
func ToSchemaMessage(msg *entity.Message) *schema.Message {
	sm := &schema.Message{
		Role:       toSchemaRole(msg.Role),
		Content:    msg.Content,
		Name:       msg.Name,
		ToolCallID: msg.ToolCallID,
	}

	if len(msg.ToolCalls) > 0 {
		sm.ToolCalls = make([]schema.ToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			sm.ToolCalls = append(sm.ToolCalls, schema.ToolCall{
				ID: tc.ID,
				Function: schema.FunctionCall{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
	}
	return sm
}

func FromSchemaMessage(sm *schema.Message) *entity.Message {
	if sm == nil {
		return nil
	}
	msg := &entity.Message{
		Role:       fromSchemaRole(sm.Role),
		Content:    sm.Content,
		Name:       sm.Name,
		ToolCallID: sm.ToolCallID,
	}

	if len(sm.ToolCalls) > 0 {
		msg.ToolCalls = make([]*entity.ToolCall, 0, len(sm.ToolCalls))
		for _, tc := range sm.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, &entity.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
	}
	return msg
}

func FromSchemaMessages(sms []*schema.Message) []*entity.Message {
	result := make([]*entity.Message, 0, len(sms))
	for _, sm := range sms {
		if m := FromSchemaMessage(sm); m != nil {
			result = append(result, FromSchemaMessage(sm))
		}
	}
	return result
}

// toSchemaRole converts domain role to Eino schema role.
func toSchemaRole(role entity.Role) schema.RoleType {
	switch role {
	case entity.RoleUser:
		return schema.User
	case entity.RoleAssistant:
		return schema.Assistant
	case entity.RoleSystem:
		return schema.System
	case entity.RoleTool:
		return schema.Tool
	default:
		return schema.User
	}
}

// fromSchemaRole converts Eino schema role to domain role.
func fromSchemaRole(role schema.RoleType) entity.Role {
	switch role {
	case schema.User:
		return entity.RoleUser
	case schema.Assistant:
		return entity.RoleAssistant
	case schema.System:
		return entity.RoleSystem
	case schema.Tool:
		return entity.RoleTool
	default:
		return entity.RoleUser
	}
}
