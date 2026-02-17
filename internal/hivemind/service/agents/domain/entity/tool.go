package entity

// ToolCall represents an LLM's request to execute a tool.
type ToolCall struct {
	// ID is the unique identifier for the tool call.
	ID string `json:"id"`
	// Name is the tool name to invoke
	Name string `json:"name"`
	// Arguments is the JSON string of the tool arguments.
	Arguments string `json:"arguments"`
}

// ToolResult represents the result of a tool call.
type ToolResult struct {
	// ToolCallID is the ID of the tool call this result corresponds to.
	ToolCallID string `json:"tool_call_id"`
	// Name is the tool name that was invoked.
	Name string `json:"name"`
	// Content is the JSON string of the tool result content.
	Content string `json:"content"`
	// Error is the error message if the tool call failed.
	Error string `json:"error"`
}
