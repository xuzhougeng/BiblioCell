package library

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestConversationsRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/bibliocell.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	saved, err := store.SaveConversation(ctx, "", "", json.RawMessage(`[{"role":"user","content":"  库里有多少   文献？ "},{"role":"assistant","content":"886 篇"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(saved.ID, "C-") || saved.Title != "库里有多少 文献？" || saved.MessageCount != 2 || saved.CreatedAt == "" {
		t.Fatalf("saved = %+v", saved)
	}
	// Saving again under the same ID replaces the transcript and keeps the ID.
	updated, err := store.SaveConversation(ctx, saved.ID, "改名", json.RawMessage(`[{"role":"user","content":"a"},{"role":"assistant","content":"b"},{"role":"user","content":"c"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != saved.ID || updated.Title != "改名" || updated.MessageCount != 3 || updated.CreatedAt != saved.CreatedAt {
		t.Fatalf("updated = %+v", updated)
	}
	second, err := store.SaveConversation(ctx, "", strings.Repeat("长", 200), json.RawMessage(`[{"role":"user","content":"x"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(second.Title)); n != maxConversationTitle+1 {
		t.Fatalf("title length = %d", n)
	}

	list, err := store.ListConversations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].Title != "改名" {
		t.Fatalf("list = %+v", list)
	}
	got, err := store.GetConversation(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var messages []map[string]string
	if err := json.Unmarshal(got.Messages, &messages); err != nil || len(messages) != 3 || messages[2]["content"] != "c" {
		t.Fatalf("messages = %s (%v)", got.Messages, err)
	}

	for _, bad := range []string{`{"role":"user"}`, `[]`, `not json`} {
		if _, err := store.SaveConversation(ctx, "", "t", json.RawMessage(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	if err := store.DeleteConversation(ctx, saved.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteConversation(ctx, saved.ID); err == nil {
		t.Fatal("deleted a missing conversation")
	}
	if list, _ := store.ListConversations(ctx); len(list) != 1 {
		t.Fatalf("list after delete = %+v", list)
	}
}
