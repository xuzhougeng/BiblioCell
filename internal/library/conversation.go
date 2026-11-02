package library

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Conversation is one saved exchange with the built-in assistant. Messages is
// the transcript as the agent package encodes it, kept opaque here so the
// library does not depend on it.
type Conversation struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Messages     json.RawMessage `json:"messages"`
	MessageCount int             `json:"messageCount"`
	CreatedAt    string          `json:"createdAt"`
	UpdatedAt    string          `json:"updatedAt"`
}

// ConversationSummary lists a conversation without its transcript.
type ConversationSummary struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	MessageCount int    `json:"messageCount"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

const maxConversationTitle = 120

// ListConversations returns saved conversations, newest first.
func (s *Store) ListConversations(ctx context.Context) ([]ConversationSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, message_count, created_at, updated_at
		FROM agent_conversations ORDER BY updated_at DESC, id LIMIT 500`)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()
	conversations := []ConversationSummary{}
	for rows.Next() {
		var c ConversationSummary
		if err := rows.Scan(&c.ID, &c.Title, &c.MessageCount, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("read conversation: %w", err)
		}
		conversations = append(conversations, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	return conversations, nil
}

// GetConversation returns one conversation with its transcript.
func (s *Store) GetConversation(ctx context.Context, id string) (Conversation, error) {
	var c Conversation
	var messages string
	err := s.db.QueryRowContext(ctx, `SELECT id, title, messages, message_count, created_at, updated_at
		FROM agent_conversations WHERE id = ?`, id).Scan(&c.ID, &c.Title, &messages, &c.MessageCount, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Conversation{}, fmt.Errorf("conversation %q not found", id)
	}
	if err != nil {
		return Conversation{}, fmt.Errorf("read conversation: %w", err)
	}
	c.Messages = json.RawMessage(messages)
	return c, nil
}

// SaveConversation stores a transcript under id, creating the conversation
// when id is empty. The title is trimmed to a line; a blank one is derived
// from the first message text.
func (s *Store) SaveConversation(ctx context.Context, id, title string, messages json.RawMessage) (Conversation, error) {
	var entries []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(messages, &entries); err != nil {
		return Conversation{}, fmt.Errorf("messages must be a JSON array of turns: %w", err)
	}
	if len(entries) == 0 {
		return Conversation{}, errors.New("a conversation needs at least one message")
	}
	title = strings.TrimSpace(strings.Join(strings.Fields(title), " "))
	if title == "" {
		for _, entry := range entries {
			if entry.Role == "user" && strings.TrimSpace(entry.Content) != "" {
				title = strings.Join(strings.Fields(entry.Content), " ")
				break
			}
		}
	}
	if title == "" {
		title = "未命名对话"
	}
	if runes := []rune(title); len(runes) > maxConversationTitle {
		title = string(runes[:maxConversationTitle]) + "…"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if id == "" {
		var suffix [4]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return Conversation{}, fmt.Errorf("generate conversation ID: %w", err)
		}
		id = fmt.Sprintf("C-%d-%s", time.Now().UnixMilli(), hex.EncodeToString(suffix[:]))
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_conversations(id, title, messages, message_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET title = excluded.title, messages = excluded.messages,
			message_count = excluded.message_count, updated_at = excluded.updated_at`,
		id, title, string(messages), len(entries), now, now)
	if err != nil {
		return Conversation{}, fmt.Errorf("save conversation: %w", err)
	}
	return s.GetConversation(ctx, id)
}

// DeleteConversation removes a saved conversation.
func (s *Store) DeleteConversation(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM agent_conversations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("conversation %q not found", id)
	}
	return nil
}
