package llm

import (
	"strconv"
	"strings"
	"unicode"
)

func parseToolCallsFromText(text string) []ToolCall {
	var calls []ToolCall
	lines := strings.Split(text, "\n")

	validTools := map[string]bool{
		"create_task":          true,
		"edit_task":            true,
		"add_details":          true,
		"add_time":             true,
		"add_commit_reference": true,
		"remove_task":          true,
		"get_codebase_context": true,
		"merge_tasks":          true,
		"split_task":           true,
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Strip leading $ or > (hallucinated CLI prompts)
		if strings.HasPrefix(line, "$ ") || strings.HasPrefix(line, "> ") {
			line = strings.TrimSpace(line[2:])
		} else if strings.HasPrefix(line, "$") {
			line = strings.TrimSpace(line[1:])
		}

		// 1. Try to find a tool name at the start of the line or after some space
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		toolName := ""
		paramsStr := ""

		// Check if first word is a tool call like name(...)
		first := parts[0]
		if idx := strings.Index(first, "("); idx > 0 {
			potentialName := first[:idx]
			if validTools[potentialName] {
				toolName = potentialName
				// Find matching paren in the rest of the line
				end, ok := findMatchingParen(line, strings.Index(line, "("))
				if ok {
					paramsStr = line[strings.Index(line, "(")+1 : end]
				}
			}
		} else if validTools[first] {
			toolName = first
			paramsStr = strings.TrimSpace(line[len(first):])
		}

		if toolName != "" {
			params := parseArgs(paramsStr)
			params = normalizeToolParams(toolName, params)
			calls = append(calls, ToolCall{
				Tool:       toolName,
				Parameters: params,
			})
		}
	}

	// Legacy fallback for embedded calls like "Inside some text tool(a=1) more text"
	if len(calls) == 0 {
		return parseToolCallsLegacy(text)
	}

	return calls
}

func parseToolCallsLegacy(text string) []ToolCall {
	var calls []ToolCall
	for i := 0; i < len(text); i++ {
		if !isNameStart(text[i]) {
			continue
		}
		start := i
		for i < len(text) && isNamePart(text[i]) {
			i++
		}
		name := text[start:i]
		j := i
		for j < len(text) && unicode.IsSpace(rune(text[j])) {
			j++
		}
		if j >= len(text) || text[j] != '(' {
			i = j
			continue
		}
		end, ok := findMatchingParen(text, j)
		if !ok {
			i = j
			continue
		}
		args := text[j+1 : end]
		params := parseArgs(args)
		params = normalizeToolParams(name, params)
		calls = append(calls, ToolCall{
			Tool:       name,
			Parameters: params,
		})
		i = end
	}
	return calls
}

func isNameStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isNamePart(b byte) bool {
	return isNameStart(b) || (b >= '0' && b <= '9')
}

func findMatchingParen(s string, start int) (int, bool) {
	depth := 0
	inSingle := false
	inDouble := false
	escape := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if escape {
			escape = false
			continue
		}
		if ch == '\\' && (inSingle || inDouble) {
			escape = true
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
		}
		if inSingle || inDouble {
			continue
		}
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return -1, false
}

func parseArgs(args string) map[string]interface{} {
	out := make(map[string]interface{})
	parts := splitArgs(args)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, val, ok := splitKeyVal(part)
		if !ok {
			continue
		}
		out[key] = parseValue(val)
	}
	return out
}

func splitArgs(s string) []string {
	var parts []string
	var buf strings.Builder
	inSingle := false
	inDouble := false
	escape := false
	depth := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escape {
			buf.WriteByte(ch)
			escape = false
			continue
		}
		if ch == '\\' && (inSingle || inDouble) {
			escape = true
			buf.WriteByte(ch)
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
		} else if ch == '"' && !inSingle {
			inDouble = !inDouble
		} else if !inSingle && !inDouble {
			if ch == '(' {
				depth++
			} else if ch == ')' && depth > 0 {
				depth--
			} else if ch == ',' && depth == 0 {
				parts = append(parts, buf.String())
				buf.Reset()
				continue
			}
		}
		buf.WriteByte(ch)
	}
	if buf.Len() > 0 {
		parts = append(parts, buf.String())
	}
	return parts
}

func splitKeyVal(part string) (string, string, bool) {
	key := ""
	val := ""
	ok := false

	if idx := strings.Index(part, "="); idx >= 0 {
		key = strings.TrimSpace(part[:idx])
		val = strings.TrimSpace(part[idx+1:])
		ok = true
	} else if idx := strings.Index(part, ":"); idx >= 0 {
		key = strings.TrimSpace(part[:idx])
		val = strings.TrimSpace(part[idx+1:])
		ok = true
	}

	if ok {
		// Strip leading dashes (CLI style)
		key = strings.TrimLeft(key, "-")
		return key, val, true
	}

	return "", "", false
}

func parseValue(val string) interface{} {
	val = strings.TrimSpace(val)
	if val == "" {
		return ""
	}
	if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
		(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
		return val[1 : len(val)-1]
	}
	low := strings.ToLower(val)
	switch low {
	case "null", "nil", "none":
		return nil
	case "true":
		return true
	case "false":
		return false
	}
	if i, err := strconv.Atoi(val); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f
	}
	return val
}

func normalizeToolParams(tool string, params map[string]interface{}) map[string]interface{} {
	// Global normalizations
	if v, ok := params["task_id"]; ok {
		if _, has := params["index"]; !has {
			params["index"] = v
		}
	}
	if v, ok := params["task_index"]; ok {
		if _, has := params["index"]; !has {
			params["index"] = v
		}
	}
	if v, ok := params["technical_why"]; ok {
		if _, has := params["details"]; !has {
			params["details"] = v
		}
	}

	switch tool {
	case "create_task", "update_task":
		if v, ok := params["task_intent"]; ok {
			if _, has := params["intent"]; !has {
				params["intent"] = v
			}
			delete(params, "task_intent")
		}
		if v, ok := params["task_type"]; ok {
			if _, has := params["type"]; !has {
				params["type"] = v
			}
			delete(params, "task_type")
		}
		if v, ok := params["id"]; ok {
			if _, has := params["index"]; !has {
				params["index"] = v
			}
			delete(params, "id")
		}
	case "merge_tasks":
		if v, ok := params["task_ids"]; ok {
			if _, has := params["indices"]; !has {
				params["indices"] = v
			}
		}
	}
	return params
}
