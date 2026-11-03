package computeruse

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func jsonUnmarshal(data []byte, target any) error {
	return json.Unmarshal(data, target)
}

func findAppPID(value any, bundleID string) (int64, bool) {
	var visit func(any) (int64, bool)
	visit = func(current any) (int64, bool) {
		switch node := current.(type) {
		case []any:
			for _, child := range node {
				if pid, ok := visit(child); ok {
					return pid, true
				}
			}
		case map[string]any:
			id := stringField(node, "bundle_id", "bundleId", "bundleIdentifier")
			pid, _ := numberField(node, "pid", "process_id", "processId")
			running, hasRunning := node["running"].(bool)
			if id == bundleID && pid > 0 && (!hasRunning || running) {
				return pid, true
			}
			for key, child := range node {
				if key == "apps" || key == "data" || key == "result" || key == "structuredContent" {
					if foundPID, ok := visit(child); ok {
						return foundPID, true
					}
				}
			}
		}
		return 0, false
	}
	return visit(value)
}

func findUniqueWindow(value any) (int64, string, bool) {
	var windows []any
	var visit func(any)
	visit = func(current any) {
		switch node := current.(type) {
		case []any:
			for _, child := range node {
				visit(child)
			}
		case map[string]any:
			if items, ok := node["windows"].([]any); ok {
				windows = append(windows, items...)
				return
			}
			for key, child := range node {
				if key == "data" || key == "result" || key == "structuredContent" {
					visit(child)
				}
			}
		}
	}
	visit(value)
	if len(windows) != 1 {
		return 0, "", false
	}
	window, ok := windows[0].(map[string]any)
	if !ok {
		return 0, "", false
	}
	id, ok := numberField(window, "window_id", "windowId", "id")
	if !ok || id < 0 {
		return 0, "", false
	}
	title := stringField(window, "title", "name")
	if title == "" {
		title = "Current window"
	}
	return id, compactString(title, 160), true
}

func matchingButtonTokens(value any, labels []string) []string {
	wanted := make(map[string]bool, len(labels))
	for _, label := range labels {
		wanted[strings.ToLower(strings.TrimSpace(label))] = true
	}
	seen := map[string]bool{}
	var result []string
	var visit func(any)
	visit = func(current any) {
		switch node := current.(type) {
		case []any:
			for _, child := range node {
				visit(child)
			}
		case map[string]any:
			label := strings.ToLower(strings.TrimSpace(stringField(node, "label", "name", "title")))
			role := strings.ToLower(strings.TrimSpace(stringField(node, "role", "type")))
		token := stringField(node, "element_token", "elementToken")
			if token != "" && wanted[label] && strings.Contains(role, "button") && !seen[token] {
				seen[token] = true
				result = append(result, token)
			}
			for _, child := range node {
				visit(child)
			}
		}
	}
	visit(value)
	return result
}

func musicLabels(action string) ([]string, []string) {
	switch action {
	case "play":
		return []string{"Play"}, []string{"Pause"}
	case "pause":
		return []string{"Pause"}, []string{"Play"}
	case "next":
		return []string{"Next", "Next Track", "Skip Forward"}, nil
	case "previous":
		return []string{"Previous", "Previous Track", "Skip Back"}, nil
	default:
		return nil, nil
	}
}

func encodeUntrustedObservation(appName, window string, state map[string]any) (string, error) {
	elements, ok := state["elements"]
	if !ok {
		if tree, found := state["accessibility_tree"]; found {
			elements = tree
		} else {
			return "", errors.New("Cua Driver did not return an accessibility tree")
		}
	}
	safe := safeTree(elements)
	encoded, err := json.Marshal(safe)
	if err != nil {
		return "", errors.New("could not encode the Apple Notes accessibility tree")
	}
	if len(encoded) > 4600 {
		encoded = append(encoded[:4600], []byte("… truncated")...)
	}
	result := map[string]any{
		"untrusted_external_data": true,
		"application":             appName,
		"window":                  compactString(window, 160),
		"accessibility_tree":      string(encoded),
	}
	output, err := json.Marshal(result)
	if err != nil || len(output) > 6400 {
		return "", errors.New("Apple Notes accessibility output exceeded the safe response limit")
	}
	return string(output), nil
}

func safeTree(value any) any {
	switch node := value.(type) {
	case []any:
		result := make([]any, 0, len(node))
		for _, child := range node {
			result = append(result, safeTree(child))
		}
		return result
	case map[string]any:
		result := make(map[string]any)
		for _, key := range []string{"role", "label", "value", "selected", "enabled"} {
			if child, ok := node[key]; ok {
				switch child.(type) {
				case string, bool, float64:
					result[key] = child
				}
			}
		}
		for _, key := range []string{"children", "elements"} {
			if child, ok := node[key]; ok {
				result[key] = safeTree(child)
			}
		}
		return result
	case string:
		return compactString(node, 900)
	default:
		return nil
	}
}

func stringField(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := value[key].(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func numberField(value map[string]any, keys ...string) (int64, bool) {
	for _, key := range keys {
		switch number := value[key].(type) {
		case float64:
			return int64(number), number == float64(int64(number))
		case int64:
			return number, true
		case int:
			return int64(number), true
		case json.Number:
			parsed, err := number.Int64()
			return parsed, err == nil
		}
	}
	return 0, false
}

func compactString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && end < len(value) && (value[end]&0xC0) == 0x80 {
		end--
	}
	return fmt.Sprintf("%s…", value[:end])
}

