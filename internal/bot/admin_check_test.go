package bot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestEvaluateAdminCheckResultReady(t *testing.T) {
	now := time.Date(2026, 8, 20, 13, 0, 0, 0, time.UTC)
	result := EvaluateAdminCheckResult("@gophers_id", tgbotapi.User{
		ID:       8941513753,
		UserName: "TeleAntiSpam2Bot",
	}, tgbotapi.ChatMember{
		Status:             "administrator",
		CanDeleteMessages:  true,
		CanRestrictMembers: true,
	}, now)

	if !result.AdminReady {
		t.Fatalf("AdminReady = false, missing %v", result.MissingRights)
	}
	if result.BotUsername != "TeleAntiSpam2Bot" {
		t.Fatalf("BotUsername = %q", result.BotUsername)
	}
	if got := result.Summary(); got == "" {
		t.Fatal("Summary is empty")
	}
}

func TestEvaluateAdminCheckResultMissingStatusAndRights(t *testing.T) {
	result := EvaluateAdminCheckResult("@gophers_id", tgbotapi.User{}, tgbotapi.ChatMember{
		Status: "member",
	}, time.Now())

	want := []string{"administrator_status", "can_delete_messages", "can_restrict_members"}
	if result.AdminReady {
		t.Fatal("AdminReady = true")
	}
	if len(result.MissingRights) != len(want) {
		t.Fatalf("missing rights len = %d, want %d: %v", len(result.MissingRights), len(want), result.MissingRights)
	}
	for i, item := range want {
		if result.MissingRights[i] != item {
			t.Fatalf("missing[%d] = %q, want %q", i, result.MissingRights[i], item)
		}
	}
}

func TestAdminCheckChatConfig(t *testing.T) {
	named := adminCheckChatConfig("gophers_id", 123)
	if named.SuperGroupUsername != "@gophers_id" {
		t.Fatalf("SuperGroupUsername = %q", named.SuperGroupUsername)
	}
	if named.UserID != 123 {
		t.Fatalf("UserID = %d", named.UserID)
	}

	numeric := adminCheckChatConfig("-1001116539442", 456)
	if numeric.ChatID != -1001116539442 {
		t.Fatalf("ChatID = %d", numeric.ChatID)
	}
	if numeric.UserID != 456 {
		t.Fatalf("UserID = %d", numeric.UserID)
	}
}

func TestWriteAdminCheckResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "status.json")
	want := AdminCheckResult{
		CheckedAt:  time.Date(2026, 8, 20, 13, 0, 0, 0, time.UTC),
		Chat:       "@gophers_id",
		AdminReady: true,
	}
	if err := WriteAdminCheckResult(path, want); err != nil {
		t.Fatalf("WriteAdminCheckResult: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got AdminCheckResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !got.AdminReady || got.Chat != want.Chat {
		t.Fatalf("got %+v", got)
	}
}
