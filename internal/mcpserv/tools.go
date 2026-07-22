package mcpserv

// Tool names. Kept as constants so handlers and the catalog can't drift.
const (
	toolGetContext     = "grove_get_context"
	toolReportProgress = "grove_report_progress"
	toolRaiseAttention = "grove_raise_attention"
	toolComplete       = "grove_complete"
	toolSendMessage    = "grove_send_message"
	toolSpawnChild     = "grove_spawn_child"
	toolListChildren   = "grove_list_children"
	toolNodeStatus     = "grove_node_status"
)

// toolDef is one MCP tool advertised by tools/list.
type toolDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema jsonSchema `json:"inputSchema"`
}

// jsonSchema is a minimal JSON Schema object describing a tool's arguments.
type jsonSchema struct {
	Type       string                `json:"type"`
	Properties map[string]schemaProp `json:"properties"`
	Required   []string              `json:"required,omitempty"`
}

type schemaProp struct {
	Type        string      `json:"type"`
	Description string      `json:"description,omitempty"`
	Items       *schemaItem `json:"items,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
}

type schemaItem struct {
	Type string `json:"type"`
}

func obj(props map[string]schemaProp, required ...string) jsonSchema {
	if props == nil {
		props = map[string]schemaProp{}
	}
	return jsonSchema{Type: "object", Properties: props, Required: required}
}

func strArray(desc string) schemaProp {
	return schemaProp{Type: "array", Description: desc, Items: &schemaItem{Type: "string"}}
}

// workerTools are available to every node.
func workerTools() []toolDef {
	return []toolDef{
		{
			Name:        toolGetContext,
			Description: "Re-orient: return this node's id, tree path, working directory, role, and remaining limits. Call after compaction or when unsure of your place in the tree.",
			InputSchema: obj(nil),
		},
		{
			Name:        toolReportProgress,
			Description: "Record a progress update (free-text summary, optional checklist and percent). Shows your status in the tree; does not interrupt anyone.",
			InputSchema: obj(map[string]schemaProp{
				"summary":   {Type: "string", Description: "one-line summary of what you just did or are doing"},
				"checklist": strArray("optional step list; prefix done items with [x]"),
				"percent":   {Type: "integer", Description: "optional completion percent 0-100"},
			}, "summary"),
		},
		{
			Name:        toolRaiseAttention,
			Description: "Ask the human for input: a question, a decision, a blocker, a review, or to report an error. Raises an inbox item on your node.",
			InputSchema: obj(map[string]schemaProp{
				"kind":    {Type: "string", Description: "why you need attention", Enum: []string{"question", "decision", "blocked", "review", "error"}},
				"message": {Type: "string", Description: "what you need from the human"},
				"options": strArray("optional choices the human can pick from"),
			}, "kind", "message"),
		},
		{
			Name:        toolComplete,
			Description: "Finish this node. The ONLY way a node is marked done — idleness never completes it. Wakes your parent if you have one.",
			InputSchema: obj(map[string]schemaProp{
				"result":    {Type: "string", Description: "outcome", Enum: []string{"done", "failed"}},
				"summary":   {Type: "string", Description: "what you accomplished or why you failed"},
				"artifacts": strArray("optional produced artifacts (paths, PR urls)"),
			}, "result", "summary"),
		},
		{
			Name:        toolSendMessage,
			Description: "Send a text message to an adjacent node — your parent, or (for orchestrators) one of your children. Wakes the target.",
			InputSchema: obj(map[string]schemaProp{
				"node_id": {Type: "string", Description: "the parent or child node id to message"},
				"text":    {Type: "string", Description: "the message"},
			}, "node_id", "text"),
		},
	}
}

// orchestratorTools are available only to orchestrator-role nodes.
func orchestratorTools() []toolDef {
	return []toolDef{
		{
			Name:        toolSpawnChild,
			Description: "Delegate work to a new child node. Asynchronous: returns immediately with status \"spawning\". After spawning, END YOUR TURN — grove wakes you when the child reports. Do not poll.",
			InputSchema: obj(map[string]schemaProp{
				"title":  {Type: "string", Description: "short title for the child node"},
				"prompt": {Type: "string", Description: "the full task briefing for the child"},
				"role":   {Type: "string", Description: "worker (default) or orchestrator if the child should spawn its own children", Enum: []string{"worker", "orchestrator"}},
				"mode":   {Type: "string", Description: "headless (default) or pty for an interactive child", Enum: []string{"headless", "pty"}},
				"driver": {Type: "string", Description: "optional CLI driver override; inherits from you by default"},
				"repos":  strArray("optional repo ids/names to attach to the child"),
			}, "title", "prompt"),
		},
		{
			Name:        toolListChildren,
			Description: "List your direct children with their status, latest progress and attention. Use when woken; do not poll in a loop.",
			InputSchema: obj(map[string]schemaProp{
				"depth": {Type: "integer", Description: "how many levels deep to include (default 1)"},
			}),
		},
		{
			Name:        toolNodeStatus,
			Description: "Get a status rollup for a node in your subtree (defaults to yourself): counts by status, attention, and completion percent.",
			InputSchema: obj(map[string]schemaProp{
				"node_id": {Type: "string", Description: "a node id in your subtree; defaults to you"},
				"subtree": {Type: "boolean", Description: "include the full subtree rollup (default true)"},
			}),
		},
	}
}

// toolCatalog returns the tools visible to a node of the given role.
func toolCatalog(role Role) []toolDef {
	tools := workerTools()
	if role.CanOrchestrate() {
		tools = append(tools, orchestratorTools()...)
	}
	return tools
}
