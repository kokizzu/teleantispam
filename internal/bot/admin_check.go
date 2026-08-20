package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	DefaultAdminCheckChat       = "@gophers_id"
	DefaultAdminCheckStatusPath = "/var/lib/teleantispam/gophers-admin-status.json"
)

type AdminCheckResult struct {
	CheckedAt          time.Time `json:"checked_at"`
	Chat               string    `json:"chat"`
	BotID              int64     `json:"bot_id,omitempty"`
	BotUsername        string    `json:"bot_username,omitempty"`
	Status             string    `json:"status,omitempty"`
	CanManageChat      bool      `json:"can_manage_chat"`
	CanDeleteMessages  bool      `json:"can_delete_messages"`
	CanRestrictMembers bool      `json:"can_restrict_members"`
	CanPromoteMembers  bool      `json:"can_promote_members"`
	AdminReady         bool      `json:"admin_ready"`
	MissingRights      []string  `json:"missing_rights,omitempty"`
	Error              string    `json:"error,omitempty"`
}

func RunAdminCheck(token string, chat string, statusPath string, now time.Time) (AdminCheckResult, error) {
	result := AdminCheckResult{
		CheckedAt: now.UTC(),
		Chat:      chat,
	}
	api, err := tgbotapi.NewBotAPI(strings.TrimSpace(token))
	if err != nil {
		result.Error = err.Error()
		_ = WriteAdminCheckResult(statusPath, result)
		return result, err
	}

	result.BotID = api.Self.ID
	result.BotUsername = api.Self.UserName

	member, err := api.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: adminCheckChatConfig(chat, api.Self.ID),
	})
	if err != nil {
		result.Error = err.Error()
		_ = WriteAdminCheckResult(statusPath, result)
		return result, err
	}

	result = EvaluateAdminCheckResult(chat, api.Self, member, now)
	if err := WriteAdminCheckResult(statusPath, result); err != nil {
		return result, err
	}
	return result, nil
}

func EvaluateAdminCheckResult(chat string, self tgbotapi.User, member tgbotapi.ChatMember, now time.Time) AdminCheckResult {
	missing := missingAdminRights(member)
	return AdminCheckResult{
		CheckedAt:          now.UTC(),
		Chat:               chat,
		BotID:              self.ID,
		BotUsername:        self.UserName,
		Status:             member.Status,
		CanManageChat:      member.CanManageChat,
		CanDeleteMessages:  member.CanDeleteMessages,
		CanRestrictMembers: member.CanRestrictMembers,
		CanPromoteMembers:  member.CanPromoteMembers,
		AdminReady:         len(missing) == 0,
		MissingRights:      missing,
	}
}

func (result AdminCheckResult) Summary() string {
	if result.Error != "" {
		return fmt.Sprintf("admin_check chat=%s ok=false error=%q", result.Chat, result.Error)
	}
	missing := "-"
	if len(result.MissingRights) > 0 {
		missing = strings.Join(result.MissingRights, ",")
	}
	username := "-"
	if result.BotUsername != "" {
		username = "@" + result.BotUsername
	}
	return fmt.Sprintf(
		"admin_check chat=%s bot=%s status=%s can_delete_messages=%t can_restrict_members=%t admin_ready=%t missing=%s",
		result.Chat,
		username,
		result.Status,
		result.CanDeleteMessages,
		result.CanRestrictMembers,
		result.AdminReady,
		missing,
	)
}

func WriteAdminCheckResult(path string, result AdminCheckResult) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".admin-check-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(result)
	closeErr := tmp.Close()
	if writeErr != nil {
		_ = os.Remove(tmpName)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpName)
		return closeErr
	}
	if err := os.Chmod(tmpName, 0o640); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func missingAdminRights(member tgbotapi.ChatMember) []string {
	var missing []string
	if member.Status != "administrator" && member.Status != "creator" {
		missing = append(missing, "administrator_status")
	}
	if !member.CanDeleteMessages {
		missing = append(missing, "can_delete_messages")
	}
	if !member.CanRestrictMembers {
		missing = append(missing, "can_restrict_members")
	}
	return missing
}

func adminCheckChatConfig(chat string, userID int64) tgbotapi.ChatConfigWithUser {
	chat = strings.TrimSpace(chat)
	config := tgbotapi.ChatConfigWithUser{UserID: userID}
	if chatID, err := strconv.ParseInt(chat, 10, 64); err == nil {
		config.ChatID = chatID
		return config
	}
	if !strings.HasPrefix(chat, "@") {
		chat = "@" + chat
	}
	config.SuperGroupUsername = chat
	return config
}
