package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatHandler_UnauthorizedWithoutToken(t *testing.T) {
	repo := &Repository{tableName: "TestTable"}
	handler := NewHandler(repo)

	req := httptest.NewRequest("GET", "/chat/messages?peer=test@example.com", nil)
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", rec.Code)
	}
}

func TestChatMessageModel(t *testing.T) {
	msg := Message{
		ID:             "msg-123",
		SenderEmail:    "alice@example.com",
		SenderID:       "user-alice",
		RecipientEmail: "bob@example.com",
		Text:           "test message",
		Time:           "2026-10-06T19:00:00Z",
		Delivered:      true,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	if decoded.ID != "msg-123" || decoded.Text != "test message" {
		t.Fatalf("mismatched decoded message content: %+v", decoded)
	}
}
