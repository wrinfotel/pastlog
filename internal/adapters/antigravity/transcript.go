package antigravity

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

type call struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type step struct {
	Index      *int64 `json:"step_index"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Created    string `json:"created_at"`
	Content    string `json:"content"`
	Thinking   string `json:"thinking"`
	CWD        string `json:"cwd"`
	Model      string `json:"model"`
	Calls      []call `json:"tool_calls"`
	Input      int64  `json:"input_tokens"`
	Output     int64  `json:"output_tokens"`
	CacheRead  int64  `json:"cache_read_tokens"`
	CacheWrite int64  `json:"cache_write_tokens"`
}

func (s step) timestamp() time.Time {
	return parseTime(s.Created)
}

func parseTime(value string) time.Time {
	if at, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return at
	}
	return time.Time{}
}

func (s step) usage() agentlog.Usage {
	if s.Status != "" && s.Status != "DONE" {
		return agentlog.Usage{}
	}
	return agentlog.Usage{Input: s.Input + s.CacheRead + s.CacheWrite, CacheRead: s.CacheRead, CacheWrite: s.CacheWrite, Output: s.Output}
}

func (s step) messageRole() string {
	switch s.Type {
	case "USER_INPUT":
		return "user"
	case "PLANNER_RESPONSE":
		return "assistant"
	case "ERROR_MESSAGE", "SYSTEM_MESSAGE":
		return "system"
	default:
		return ""
	}
}

func (s step) messageText() string {
	if s.Type == "USER_INPUT" {
		return userText(s.Content)
	}
	return s.Content
}

func userText(content string) string {
	const start = "<USER_REQUEST>"
	if value, ok := strings.CutPrefix(strings.TrimSpace(content), start); ok {
		if request, _, found := strings.Cut(value, "</USER_REQUEST>"); found {
			return strings.TrimSpace(request)
		}
	}
	return content
}

var childID = regexp.MustCompile(`"conversationId"\s*:\s*"([a-fA-F0-9]{8}(?:-[a-fA-F0-9]{4}){3}-[a-fA-F0-9]{12})"`)

func (s step) children() []string {
	if s.Type != "INVOKE_SUBAGENT" {
		return nil
	}
	var ids []string
	for _, match := range childID.FindAllStringSubmatch(s.Content, -1) {
		ids = append(ids, match[1])
	}
	return ids
}

func (s step) resultTool() string {
	switch s.Type {
	case "RUN_COMMAND", "COMMAND_STATUS", "SEND_COMMAND_INPUT", "VIEW_FILE", "VIEW_CODE_ITEM", "LIST_DIRECTORY",
		"GREP_SEARCH", "FIND_BY_NAME", "SEARCH_WEB", "READ_URL_CONTENT", "CODE_ACTION", "WRITE_TO_FILE",
		"REPLACE_FILE_CONTENT", "MULTI_REPLACE_FILE_CONTENT", "APPLY_PATCH", "MCP_TOOL", "MCP_TOOL_CALL",
		"INVOKE_SUBAGENT", "WAIT", "BROWSER_SUBAGENT", "MANAGE_TASK", "MANAGE_INBOX", "NOTIFY_USER":
		return strings.ToLower(s.Type)
	default:
		return ""
	}
}

// The CLI's tool-specific casing and display labels are irrelevant to
// context analysis. Standard keys let the shared formatter pick a path or
// command as the finding label while preserving all actual arguments.
func contextArgs(raw json.RawMessage) json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return raw
	}
	delete(fields, "toolAction")
	delete(fields, "toolSummary")
	for original, standardized := range map[string]string{"AbsolutePath": "path", "CommandLine": "command", "Pattern": "pattern", "Query": "query"} {
		if value, ok := fields[original]; ok && fields[standardized] == nil {
			fields[standardized] = value
			delete(fields, original)
		}
	}
	if data, err := json.Marshal(fields); err == nil {
		return data
	}
	return raw
}

func (s step) entries(iter func(agentlog.Entry) error) error {
	at := s.timestamp()
	send := func(kind agentlog.EntryKind, role, content string) error {
		if content == "" {
			return nil
		}
		return iter(agentlog.Entry{Kind: kind, Role: role, Text: content, Timestamp: at})
	}
	if role := s.messageRole(); role != "" {
		if err := send(agentlog.Message, role, s.messageText()); err != nil {
			return err
		}
	}
	if s.Type == "PLANNER_RESPONSE" {
		if err := send(agentlog.Summary, "assistant", s.Thinking); err != nil {
			return err
		}
		for _, tool := range s.Calls {
			text := agentlog.CompactJSON(tool.Args)
			if text == "" {
				text = tool.Name
			}
			if err := send(agentlog.ToolCall, "tool", text); err != nil {
				return err
			}
		}
	}
	if s.resultTool() != "" {
		return send(agentlog.ToolResult, "tool", s.Content)
	}
	return nil
}
