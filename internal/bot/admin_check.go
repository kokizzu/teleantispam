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
	ReadyNotified      bool      `json:"ready_notified"`
	ReadyNotifyChat    string    `json:"ready_notify_chat,omitempty"`
	ReadyNotifyError   string    `json:"ready_notify_error,omitempty"`
	RevokedNotified    bool      `json:"revoked_notified"`
	RevokedNotifyChat  string    `json:"revoked_notify_chat,omitempty"`
	RevokedNotifyError string    `json:"revoked_notify_error,omitempty"`
	Error              string    `json:"error,omitempty"`
}

func RunAdminCheck(token string, chat string, statusPath string, notifyChat string, now time.Time) (AdminCheckResult, error) {
	result := AdminCheckResult{
		CheckedAt: now.UTC(),
		Chat:      chat,
	}
	previous, _ := ReadAdminCheckResult(statusPath)

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
	notifyTarget := adminCheckNotifyChat(notifyChat, chat)
	if shouldNotifyAdminReady(previous, result) {
		result.ReadyNotifyChat = notifyTarget
		if err := sendAdminReadyNotification(api, result.ReadyNotifyChat, result); err != nil {
			result.ReadyNotifyError = err.Error()
		} else {
			result.ReadyNotified = true
		}
	} else if previous.ReadyNotified {
		result.ReadyNotified = true
		result.ReadyNotifyChat = previous.ReadyNotifyChat
	}

	if shouldNotifyAdminRevoked(previous, result) {
		result.RevokedNotifyChat = notifyTarget
		if err := sendAdminRevokedNotification(api, result.RevokedNotifyChat, result); err != nil {
			result.RevokedNotifyError = err.Error()
		} else {
			result.RevokedNotified = true
		}
	} else if previous.RevokedNotified && !result.AdminReady {
		result.RevokedNotified = true
		result.RevokedNotifyChat = previous.RevokedNotifyChat
	}

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

func ReadAdminCheckResult(path string) (AdminCheckResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return AdminCheckResult{}, os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return AdminCheckResult{}, err
	}
	var result AdminCheckResult
	if err := json.Unmarshal(data, &result); err != nil {
		return AdminCheckResult{}, err
	}
	return result, nil
}

func shouldNotifyAdminReady(previous AdminCheckResult, current AdminCheckResult) bool {
	return current.AdminReady && !previous.AdminReady && !previous.ReadyNotified && current.Error == ""
}

func shouldNotifyAdminRevoked(previous AdminCheckResult, current AdminCheckResult) bool {
	return previous.AdminReady && !current.AdminReady && !current.RevokedNotified && current.Error == ""
}

func sendAdminReadyNotification(api *tgbotapi.BotAPI, chat string, result AdminCheckResult) error {
	_, err := api.Send(adminCheckMessageConfig(chat, adminReadyNotificationText(result)))
	return err
}

func sendAdminRevokedNotification(api *tgbotapi.BotAPI, chat string, result AdminCheckResult) error {
	_, err := api.Send(adminCheckMessageConfig(chat, adminRevokedNotificationText(result)))
	return err
}

func adminReadyNotificationText(result AdminCheckResult) string {
	username := "-"
	if result.BotUsername != "" {
		username = "@" + result.BotUsername
	}
	return fmt.Sprintf("%s is admin-ready in %s: delete messages and ban/restrict users rights are present. Live test can be run.", username, result.Chat)
}

func adminRevokedNotificationText(result AdminCheckResult) string {
	username := "-"
	if result.BotUsername != "" {
		username = "@" + result.BotUsername
	}
	missing := "-"
	if len(result.MissingRights) > 0 {
		missing = strings.Join(result.MissingRights, ",")
	}
	return fmt.Sprintf("%s is no longer admin-ready in %s: missing %s. Please have a promoter-capable admin restore delete messages and ban/restrict users rights.", username, result.Chat, missing)
}

func adminCheckNotifyChat(notifyChat string, checkChat string) string {
	notifyChat = strings.TrimSpace(notifyChat)
	if notifyChat != "" {
		return notifyChat
	}
	return strings.TrimSpace(checkChat)
}

func adminCheckMessageConfig(chat string, text string) tgbotapi.MessageConfig {
	chat = strings.TrimSpace(chat)
	if chatID, err := strconv.ParseInt(chat, 10, 64); err == nil {
		return tgbotapi.NewMessage(chatID, text)
	}
	if !strings.HasPrefix(chat, "@") {
		chat = "@" + chat
	}
	return tgbotapi.NewMessageToChannel(chat, text)
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
