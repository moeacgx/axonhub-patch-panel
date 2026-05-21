package normalize

import (
	"encoding/json"
	"testing"
)

func TestCanonicalizeOpenAIChatMessages(t *testing.T) {
	body := []byte(`{
		"model":"gpt-4.1",
		"messages":[
			{"role":"system","content":"  Be useful.  "},
			{"role":"user","content":[{"type":"text","text":"Hello"},{"type":"image_url","image_url":{"url":"ignored"}}]},
			{"role":"assistant","content":"Hi there"},
			{"role":"user","content":"Continue"}
		]
	}`)

	got, err := Canonicalize(body, "/v1/chat/completions")
	if err != nil {
		t.Fatalf("Canonicalize returned error: %v", err)
	}

	want := []Message{
		{Role: "system", Text: "Be useful."},
		{Role: "user", Text: "Hello"},
		{Role: "assistant", Text: "Hi there"},
		{Role: "user", Text: "Continue"},
	}

	if !equalMessages(got.Messages, want) {
		t.Fatalf("messages mismatch\nwant: %#v\n got: %#v", want, got.Messages)
	}
	if got.Format != FormatOpenAIChat {
		t.Fatalf("format mismatch: %s", got.Format)
	}
}

func TestCanonicalizeAnthropicMessages(t *testing.T) {
	body := []byte(`{
		"system":[{"type":"text","text":"Use short answers."}],
		"messages":[
			{"role":"user","content":"First"},
			{"role":"assistant","content":[{"type":"text","text":"Second"}]},
			{"role":"user","content":"Third"}
		]
	}`)

	got, err := Canonicalize(body, "/v1/messages")
	if err != nil {
		t.Fatalf("Canonicalize returned error: %v", err)
	}

	want := []Message{
		{Role: "system", Text: "Use short answers."},
		{Role: "user", Text: "First"},
		{Role: "assistant", Text: "Second"},
		{Role: "user", Text: "Third"},
	}

	if !equalMessages(got.Messages, want) {
		t.Fatalf("messages mismatch\nwant: %#v\n got: %#v", want, got.Messages)
	}
	if got.Format != FormatAnthropicMessages {
		t.Fatalf("format mismatch: %s", got.Format)
	}
}

func TestLookupHashDropsLastUserTurn(t *testing.T) {
	doc := Document{
		Format: FormatOpenAIChat,
		Messages: []Message{
			{Role: "user", Text: "First"},
			{Role: "assistant", Text: "Second"},
			{Role: "user", Text: "Third"},
		},
	}

	before, err := HashMessages(doc.Messages[:2])
	if err != nil {
		t.Fatalf("HashMessages returned error: %v", err)
	}

	got, err := LookupHash(doc)
	if err != nil {
		t.Fatalf("LookupHash returned error: %v", err)
	}

	if got != before {
		t.Fatalf("lookup hash should match context before last user turn\nwant: %s\n got: %s", before, got)
	}
}

func TestStateHashIncludesAssistantResponse(t *testing.T) {
	doc := Document{
		Format: FormatOpenAIChat,
		Messages: []Message{
			{Role: "user", Text: "First"},
		},
	}

	responseBody := []byte(`{"id":"resp_1","choices":[{"message":{"role":"assistant","content":"Second"}}]}`)
	next, responseID, err := StateAfterResponse(doc, responseBody)
	if err != nil {
		t.Fatalf("StateAfterResponse returned error: %v", err)
	}

	wantMessages := []Message{
		{Role: "user", Text: "First"},
		{Role: "assistant", Text: "Second"},
	}
	wantHash, err := HashMessages(wantMessages)
	if err != nil {
		t.Fatalf("HashMessages returned error: %v", err)
	}

	if next != wantHash {
		t.Fatalf("state hash mismatch\nwant: %s\n got: %s", wantHash, next)
	}
	if responseID != "resp_1" {
		t.Fatalf("response id mismatch: %s", responseID)
	}
}

func TestStateAfterStreamCollectsOpenAIChatDeltas(t *testing.T) {
	doc := Document{
		Format: FormatOpenAIChat,
		Messages: []Message{
			{Role: "user", Text: "First"},
		},
	}
	streamBody := []byte("data: {\"id\":\"chatcmpl_1\",\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"Sec\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"ond\"}}]}\n\n" +
		"data: [DONE]\n\n")

	next, responseID, err := StateAfterStream(doc, streamBody)
	if err != nil {
		t.Fatalf("StateAfterStream returned error: %v", err)
	}

	wantHash, err := HashMessages([]Message{
		{Role: "user", Text: "First"},
		{Role: "assistant", Text: "Second"},
	})
	if err != nil {
		t.Fatalf("HashMessages returned error: %v", err)
	}
	if next != wantHash {
		t.Fatalf("state hash mismatch\nwant: %s\n got: %s", wantHash, next)
	}
	if responseID != "chatcmpl_1" {
		t.Fatalf("response id mismatch: %s", responseID)
	}
}

func TestStateAfterStreamCollectsOpenAIResponseDeltas(t *testing.T) {
	doc := Document{
		Format: FormatOpenAIResponses,
		Messages: []Message{
			{Role: "user", Text: "First"},
		},
	}
	streamBody := []byte("event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"Sec\"}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ond\"}\n\n")

	next, responseID, err := StateAfterStream(doc, streamBody)
	if err != nil {
		t.Fatalf("StateAfterStream returned error: %v", err)
	}

	wantHash, err := HashMessages([]Message{
		{Role: "user", Text: "First"},
		{Role: "assistant", Text: "Second"},
	})
	if err != nil {
		t.Fatalf("HashMessages returned error: %v", err)
	}
	if next != wantHash {
		t.Fatalf("state hash mismatch\nwant: %s\n got: %s", wantHash, next)
	}
	if responseID != "resp_1" {
		t.Fatalf("response id mismatch: %s", responseID)
	}
}

func TestStateAfterStreamCollectsAnthropicTextDeltas(t *testing.T) {
	doc := Document{
		Format: FormatAnthropicMessages,
		Messages: []Message{
			{Role: "user", Text: "First"},
		},
	}
	streamBody := []byte("event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Sec\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ond\"}}\n\n")

	next, responseID, err := StateAfterStream(doc, streamBody)
	if err != nil {
		t.Fatalf("StateAfterStream returned error: %v", err)
	}

	wantHash, err := HashMessages([]Message{
		{Role: "user", Text: "First"},
		{Role: "assistant", Text: "Second"},
	})
	if err != nil {
		t.Fatalf("HashMessages returned error: %v", err)
	}
	if next != wantHash {
		t.Fatalf("state hash mismatch\nwant: %s\n got: %s", wantHash, next)
	}
	if responseID != "msg_1" {
		t.Fatalf("response id mismatch: %s", responseID)
	}
}

func TestCanonicalJSONIsStable(t *testing.T) {
	msgs := []Message{
		{Role: "user", Text: " hello \n world "},
	}
	encoded, err := canonicalJSON(msgs)
	if err != nil {
		t.Fatalf("canonicalJSON returned error: %v", err)
	}

	var decoded []Message
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded[0].Text != "hello world" {
		t.Fatalf("text not normalized: %q", decoded[0].Text)
	}
}

func equalMessages(a, b []Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
