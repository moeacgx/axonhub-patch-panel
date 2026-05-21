package rewrite

import (
	"bytes"
	"encoding/json"
	"path"
	"strings"
)

type ClaudeThinkingOptions struct {
	Enabled bool
	Models  []string
	Effort  string
}

func RewriteClaudeThinking(body []byte, requestPath string, opts ClaudeThinkingOptions) ([]byte, bool, error) {
	if !opts.Enabled || !isAnthropicMessagesPath(requestPath) {
		return body, false, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, false, nil
	}
	if !modelAllowed(stringValue(payload["model"]), opts.Models) {
		return body, false, nil
	}

	thinking, ok := payload["thinking"].(map[string]any)
	if !ok {
		return body, false, nil
	}
	if !strings.EqualFold(stringValue(thinking["type"]), "enabled") {
		return body, false, nil
	}

	effort := strings.TrimSpace(opts.Effort)
	if effort == "" {
		effort = "xhigh"
	}

	thinking["type"] = "adaptive"
	delete(thinking, "budget_tokens")
	payload["thinking"] = thinking

	outputConfig, _ := payload["output_config"].(map[string]any)
	if outputConfig == nil {
		outputConfig = map[string]any{}
	}
	outputConfig["effort"] = effort
	payload["output_config"] = outputConfig

	encoded, err := json.Marshal(payload)
	if err != nil {
		return body, false, err
	}
	return encoded, !bytes.Equal(bytes.TrimSpace(body), encoded), nil
}

func isAnthropicMessagesPath(requestPath string) bool {
	cleanPath := strings.ToLower(path.Clean("/" + strings.TrimPrefix(requestPath, "/")))
	return strings.HasSuffix(cleanPath, "/messages")
}

func modelAllowed(model string, models []string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	if len(models) == 0 {
		return true
	}
	for _, allowed := range models {
		if strings.EqualFold(strings.TrimSpace(allowed), model) {
			return true
		}
	}
	return false
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
