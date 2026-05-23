package normalize

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

const (
	FormatUnknown           Format = "unknown"
	FormatOpenAIChat        Format = "openai_chat"
	FormatOpenAIResponses   Format = "openai_responses"
	FormatAnthropicMessages Format = "anthropic_messages"
)

var whitespaceRE = regexp.MustCompile(`\s+`)

type Format string

type Message struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type Document struct {
	Format     Format
	Messages   []Message
	ResponseID string
	SessionID  string
}

func Canonicalize(body []byte, requestPath string) (Document, error) {
	format := detectFormat(body, requestPath)

	switch format {
	case FormatOpenAIChat:
		return canonicalizeOpenAIChat(body)
	case FormatOpenAIResponses:
		return canonicalizeOpenAIResponses(body)
	case FormatAnthropicMessages:
		return canonicalizeAnthropic(body)
	default:
		return Document{Format: FormatUnknown}, nil
	}
}

func LookupHash(doc Document) (string, error) {
	if firstTurn := firstUserTurnOnly(doc.Messages); len(firstTurn) > 0 {
		return HashMessages(firstTurn)
	}
	messages := removeLastUserTurn(doc.Messages)
	if len(messages) == 0 {
		messages = doc.Messages
	}
	return HashMessages(messages)
}

func HashMessages(messages []Message) (string, error) {
	encoded, err := canonicalJSON(messages)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func StateAfterResponse(doc Document, responseBody []byte) (string, string, error) {
	responseID, assistantMessages, err := assistantMessagesFromResponse(responseBody)
	if err != nil {
		return "", "", err
	}
	return stateAfterAssistantMessages(doc, responseID, assistantMessages)
}

func StateAfterStream(doc Document, streamBody []byte) (string, string, error) {
	responseID, assistantMessages, err := assistantMessagesFromStream(streamBody)
	if err != nil {
		return "", "", err
	}
	return stateAfterAssistantMessages(doc, responseID, assistantMessages)
}

func stateAfterAssistantMessages(doc Document, responseID string, assistantMessages []Message) (string, string, error) {
	if len(assistantMessages) == 0 {
		return "", responseID, nil
	}
	messages := append([]Message{}, doc.Messages...)
	messages = append(messages, assistantMessages...)
	hash, err := HashMessages(messages)
	if err != nil {
		return "", "", err
	}
	return hash, responseID, nil
}

func canonicalJSON(messages []Message) ([]byte, error) {
	normalized := make([]Message, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		text := normalizeText(msg.Text)
		if role == "" || text == "" {
			continue
		}
		normalized = append(normalized, Message{Role: role, Text: text})
	}
	return json.Marshal(normalized)
}

func detectFormat(body []byte, requestPath string) Format {
	cleanPath := strings.ToLower(path.Clean("/" + strings.TrimPrefix(requestPath, "/")))
	if strings.HasSuffix(cleanPath, "/chat/completions") {
		return FormatOpenAIChat
	}
	if strings.HasSuffix(cleanPath, "/responses") {
		return FormatOpenAIResponses
	}
	if strings.HasSuffix(cleanPath, "/messages") {
		return FormatAnthropicMessages
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return FormatUnknown
	}
	if _, ok := raw["messages"]; ok {
		if _, hasAnthropicVersion := raw["anthropic_version"]; hasAnthropicVersion {
			return FormatAnthropicMessages
		}
		return FormatOpenAIChat
	}
	if _, ok := raw["input"]; ok {
		return FormatOpenAIResponses
	}
	return FormatUnknown
}

func canonicalizeOpenAIChat(body []byte) (Document, error) {
	var payload struct {
		Messages []rawMessage `json:"messages"`
		User     string       `json:"user"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Document{}, err
	}
	return Document{
		Format:    FormatOpenAIChat,
		Messages:  normalizeRawMessages(payload.Messages),
		SessionID: strings.TrimSpace(payload.User),
	}, nil
}

func canonicalizeOpenAIResponses(body []byte) (Document, error) {
	var payload struct {
		Input              json.RawMessage `json:"input"`
		Instructions       string          `json:"instructions"`
		PreviousResponseID string          `json:"previous_response_id"`
		User               string          `json:"user"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Document{}, err
	}

	var messages []Message
	if payload.Instructions != "" {
		messages = append(messages, Message{Role: "system", Text: payload.Instructions})
	}
	messages = append(messages, normalizeResponsesInput(payload.Input)...)

	return Document{
		Format:     FormatOpenAIResponses,
		Messages:   messages,
		ResponseID: strings.TrimSpace(payload.PreviousResponseID),
		SessionID:  strings.TrimSpace(payload.User),
	}, nil
}

func canonicalizeAnthropic(body []byte) (Document, error) {
	var payload struct {
		System   json.RawMessage `json:"system"`
		Messages []rawMessage    `json:"messages"`
		Metadata struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Document{}, err
	}

	var messages []Message
	systemText := extractContentText(payload.System)
	if systemText != "" {
		messages = append(messages, Message{Role: "system", Text: systemText})
	}
	messages = append(messages, normalizeRawMessages(payload.Messages)...)

	return Document{
		Format:    FormatAnthropicMessages,
		Messages:  messages,
		SessionID: strings.TrimSpace(payload.Metadata.UserID),
	}, nil
}

type rawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func normalizeRawMessages(raw []rawMessage) []Message {
	messages := make([]Message, 0, len(raw))
	for _, msg := range raw {
		text := extractContentText(msg.Content)
		if text == "" {
			continue
		}
		messages = append(messages, Message{Role: msg.Role, Text: text})
	}
	return messages
}

func normalizeResponsesInput(input json.RawMessage) []Message {
	input = bytes.TrimSpace(input)
	if len(input) == 0 || bytes.Equal(input, []byte("null")) {
		return nil
	}

	var text string
	if err := json.Unmarshal(input, &text); err == nil {
		if text = normalizeText(text); text != "" {
			return []Message{{Role: "user", Text: text}}
		}
		return nil
	}

	var messages []rawMessage
	if err := json.Unmarshal(input, &messages); err == nil {
		return normalizeRawMessages(messages)
	}

	var items []struct {
		Role    string          `json:"role"`
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(input, &items); err != nil {
		return nil
	}
	out := make([]Message, 0, len(items))
	for _, item := range items {
		role := item.Role
		if role == "" && strings.HasPrefix(item.Type, "message") {
			role = "user"
		}
		text := extractContentText(item.Content)
		if text != "" {
			out = append(out, Message{Role: role, Text: text})
		}
	}
	return out
}

func extractContentText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return normalizeText(text)
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, part := range parts {
			if part.Text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(part.Text)
		}
		return normalizeText(b.String())
	}

	var object struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &object); err == nil {
		return normalizeText(object.Text)
	}

	return ""
}

func removeLastUserTurn(messages []Message) []Message {
	idx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "user") {
			idx = i
			break
		}
	}
	if idx < 0 {
		return messages
	}
	out := make([]Message, 0, len(messages)-1)
	out = append(out, messages[:idx]...)
	out = append(out, messages[idx+1:]...)
	return out
}

func firstUserTurnOnly(messages []Message) []Message {
	if hasAssistantMessage(messages) {
		return nil
	}
	for _, msg := range messages {
		if strings.EqualFold(msg.Role, "user") {
			return []Message{{Role: "user", Text: msg.Text}}
		}
	}
	return nil
}

func hasAssistantMessage(messages []Message) bool {
	for _, msg := range messages {
		if strings.EqualFold(msg.Role, "assistant") {
			return true
		}
	}
	return false
}

func assistantMessagesFromResponse(body []byte) (string, []Message, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return "", nil, nil
	}

	var payload struct {
		ID      string `json:"id"`
		Choices []struct {
			Message *rawMessage `json:"message"`
		} `json:"choices"`
		Output []struct {
			Type    string          `json:"type"`
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"output"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", nil, err
	}

	var messages []Message
	for _, choice := range payload.Choices {
		if choice.Message == nil {
			continue
		}
		text := extractContentText(choice.Message.Content)
		if text != "" {
			role := choice.Message.Role
			if role == "" {
				role = "assistant"
			}
			messages = append(messages, Message{Role: role, Text: text})
		}
	}
	for _, item := range payload.Output {
		text := extractContentText(item.Content)
		if text == "" {
			continue
		}
		role := item.Role
		if role == "" {
			role = "assistant"
		}
		messages = append(messages, Message{Role: role, Text: text})
	}
	if len(messages) == 0 {
		text := extractContentText(payload.Content)
		if text != "" {
			messages = append(messages, Message{Role: "assistant", Text: text})
		}
	}
	return strings.TrimSpace(payload.ID), messages, nil
}

func assistantMessagesFromStream(body []byte) (string, []Message, error) {
	events := sseDataLines(body)
	if len(events) == 0 {
		return "", nil, nil
	}

	var responseID string
	var text strings.Builder
	for _, event := range events {
		if event == "[DONE]" {
			continue
		}
		id, delta, err := assistantDeltaFromStreamEvent([]byte(event))
		if err != nil {
			return "", nil, err
		}
		if responseID == "" {
			responseID = id
		}
		text.WriteString(delta)
	}

	assistantText := normalizeText(text.String())
	if assistantText == "" {
		return responseID, nil, nil
	}
	return responseID, []Message{{Role: "assistant", Text: assistantText}}, nil
}

func sseDataLines(body []byte) []string {
	body = bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	body = bytes.ReplaceAll(body, []byte("\r"), []byte("\n"))

	var events []string
	for _, block := range bytes.Split(body, []byte("\n\n")) {
		var lines []string
		for _, rawLine := range bytes.Split(block, []byte("\n")) {
			line := strings.TrimSpace(string(rawLine))
			if strings.HasPrefix(line, "data:") {
				lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if len(lines) > 0 {
			events = append(events, strings.Join(lines, "\n"))
		}
	}
	return events
}

func assistantDeltaFromStreamEvent(event []byte) (string, string, error) {
	var payload struct {
		ID      string `json:"id"`
		Choices []struct {
			Delta struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
		Type     string `json:"type"`
		Response *struct {
			ID string `json:"id"`
		} `json:"response"`
		Message *struct {
			ID string `json:"id"`
		} `json:"message"`
		Delta json.RawMessage `json:"delta"`
	}
	if err := json.Unmarshal(event, &payload); err != nil {
		return "", "", err
	}

	responseID := strings.TrimSpace(payload.ID)
	if responseID == "" && payload.Response != nil {
		responseID = strings.TrimSpace(payload.Response.ID)
	}
	if responseID == "" && payload.Message != nil {
		responseID = strings.TrimSpace(payload.Message.ID)
	}

	var b strings.Builder
	for _, choice := range payload.Choices {
		text := extractContentText(choice.Delta.Content)
		if text != "" {
			b.WriteString(text)
		}
	}
	if delta := streamDeltaText(payload.Delta); delta != "" {
		b.WriteString(delta)
	}
	return responseID, b.String(), nil
}

func streamDeltaText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}

	var object struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &object); err == nil {
		return object.Text
	}
	return ""
}

func normalizeText(text string) string {
	return strings.TrimSpace(whitespaceRE.ReplaceAllString(text, " "))
}

func RequireSupported(doc Document) error {
	if doc.Format == FormatUnknown {
		return errors.New("unsupported request format")
	}
	if len(doc.Messages) == 0 && doc.ResponseID == "" && doc.SessionID == "" {
		return fmt.Errorf("no conversation identity found")
	}
	return nil
}
