package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramClient interface {
	DeleteMessage(chatID int64, messageID int) error
	BanUser(chatID int64, userID int64) error
	SendMessage(chatID int64, text string) error
	FetchAccountEvidence(chatID int64, user tgbotapi.User) AccountEvidence
}

type botAPIClient struct {
	bot *tgbotapi.BotAPI
}

func Run(ctx context.Context, cfg Config, store *FileStore) error {
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return err
	}
	log.Printf("authorized TelegramAntiSpam as @%s", api.Self.UserName)

	client := botAPIClient{bot: api}
	nextOffset, err := drainPendingUpdates(ctx, api, cfg, store, client)
	if err != nil {
		return err
	}

	updateConfig := tgbotapi.NewUpdate(nextOffset)
	updateConfig.Timeout = cfg.PollTimeout
	updateConfig.AllowedUpdates = []string{"message"}

	updates := api.GetUpdatesChan(updateConfig)
	for {
		select {
		case <-ctx.Done():
			api.StopReceivingUpdates()
			return ctx.Err()
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			if err := HandleUpdate(cfg, store, client, update, time.Now()); err != nil {
				log.Printf("handle update failed: %v", err)
			}
		}
	}
}

func drainPendingUpdates(ctx context.Context, api *tgbotapi.BotAPI, cfg Config, store *FileStore, client TelegramClient) (int, error) {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Limit = 100
	updateConfig.Timeout = 0
	updateConfig.AllowedUpdates = []string{"message"}

	nextOffset := 0
	for {
		select {
		case <-ctx.Done():
			return nextOffset, ctx.Err()
		default:
		}

		updates, err := api.GetUpdates(updateConfig)
		if err != nil {
			return nextOffset, fmt.Errorf("startup pending update scan: %w", err)
		}
		if len(updates) == 0 {
			return nextOffset, nil
		}
		nextOffset, err = HandleUpdates(cfg, store, client, updates, time.Now())
		if err != nil {
			return nextOffset, err
		}
		updateConfig.Offset = nextOffset
		if len(updates) < updateConfig.Limit {
			return nextOffset, nil
		}
	}
}

func HandleUpdates(cfg Config, store *FileStore, client TelegramClient, updates []tgbotapi.Update, now time.Time) (int, error) {
	nextOffset := 0
	for _, update := range updates {
		if update.UpdateID >= nextOffset {
			nextOffset = update.UpdateID + 1
		}
		if err := HandleUpdate(cfg, store, client, update, now); err != nil {
			return nextOffset, err
		}
	}
	return nextOffset, nil
}

func HandleUpdate(cfg Config, store *FileStore, client TelegramClient, update tgbotapi.Update, now time.Time) error {
	if update.Message == nil {
		return nil
	}
	return HandleMessage(cfg, store, client, update.Message, now)
}

func HandleMessage(cfg Config, store *FileStore, client TelegramClient, message *tgbotapi.Message, now time.Time) error {
	if message.Chat == nil {
		return nil
	}
	chatID := message.Chat.ID
	if !cfg.AllowsChat(chatID) {
		return nil
	}

	messageTime := time.Unix(int64(message.Date), 0)
	for _, user := range message.NewChatMembers {
		if user.ID == 0 || user.IsBot {
			continue
		}
		if err := store.MarkJoin(chatID, user.ID, messageTime); err != nil {
			return fmt.Errorf("record join chat=%d user=%d: %w", chatID, user.ID, err)
		}
	}

	if message.From == nil || message.From.IsBot {
		return nil
	}

	text := message.Text
	if text == "" {
		text = message.Caption
	}

	userID := message.From.ID
	history := store.History(chatID, userID)
	decision := EvaluateMessage(cfg, history, MessageEvent{
		ChatID:    chatID,
		UserID:    userID,
		MessageID: message.MessageID,
		Text:      text,
		At:        messageTime,
		Now:       now,
	})

	if !decision.Moderate {
		return store.RecordMessage(chatID, userID, message.MessageID, messageTime, cfg.DeleteRecentLimit)
	}

	evidence := client.FetchAccountEvidence(chatID, *message.From)
	evidence.ObservedFirstSeenAt = history.FirstSeenAt
	evidence.ObservedJoinedAt = history.JoinedAt

	action := ModerationAction{
		At:                         now,
		ChatID:                     chatID,
		ChatType:                   message.Chat.Type,
		ChatTitle:                  message.Chat.Title,
		ChatUsername:               message.Chat.UserName,
		UserID:                     userID,
		User:                       evidence,
		MessageID:                  message.MessageID,
		MessageDate:                messageTime,
		MessageTextSample:          sampleText(text, 240),
		ObservedMessageCountBefore: history.MessageCount,
		ObservedFirstSeenAt:        history.FirstSeenAt,
		ObservedJoinedAt:           history.JoinedAt,
		RecentMessageIDs:           append([]int(nil), history.RecentMessageIDs...),
		Reason:                     decision.Reason,
		DeletedMessageIDs:          append([]int(nil), decision.DeleteMessageIDs...),
		DryRun:                     cfg.DryRun,
		TelegramLimitations: []string{
			"Telegram Bot API does not expose account creation date",
			"Telegram Bot API does not expose user profile description/bio for group members",
			"Telegram Bot API does not expose the client-side Report Spam action",
		},
	}

	log.Printf(
		"moderating chat=%d user=%d username=%q name=%q reason=%s posts_before=%d delete_messages=%v dry_run=%t",
		chatID,
		userID,
		message.From.UserName,
		fullName(*message.From),
		decision.Reason,
		history.MessageCount,
		decision.DeleteMessageIDs,
		cfg.DryRun,
	)

	if err := store.MarkModerated(chatID, userID, message.MessageID, messageTime, decision.Reason, cfg.DeleteRecentLimit); err != nil {
		action.Errors = append(action.Errors, "record moderation: "+err.Error())
		_ = store.AppendModerationAction(action, cfg.ActionLogLimit)
		return err
	}

	if cfg.DryRun {
		if err := store.AppendModerationAction(action, cfg.ActionLogLimit); err != nil {
			return err
		}
		return nil
	}

	deletedCount := 0
	for _, messageID := range decision.DeleteMessageIDs {
		if err := client.DeleteMessage(chatID, messageID); err != nil {
			msg := fmt.Sprintf("delete message %d: %v", messageID, err)
			action.Errors = append(action.Errors, msg)
			log.Printf("delete message failed chat=%d user=%d message=%d: %v", chatID, userID, messageID, err)
			continue
		}
		deletedCount++
	}
	action.DeletedCount = deletedCount

	if decision.BanUser {
		if err := client.BanUser(chatID, userID); err != nil {
			action.Errors = append(action.Errors, "ban user: "+err.Error())
			_ = store.AppendModerationAction(action, cfg.ActionLogLimit)
			return fmt.Errorf("ban user chat=%d user=%d: %w", chatID, userID, err)
		}
		action.Banned = true
	}

	notice := BanNotice(*message.From, decision.Reason, deletedCount)
	if err := client.SendMessage(cfg.NoticeChatID(chatID), notice); err != nil {
		action.Errors = append(action.Errors, "send ban notice: "+err.Error())
		log.Printf("send ban notice failed chat=%d user=%d: %v", chatID, userID, err)
	}

	return store.AppendModerationAction(action, cfg.ActionLogLimit)
}

func (client botAPIClient) DeleteMessage(chatID int64, messageID int) error {
	_, err := client.bot.Request(tgbotapi.NewDeleteMessage(chatID, messageID))
	return err
}

func (client botAPIClient) BanUser(chatID int64, userID int64) error {
	_, err := client.bot.Request(banUserConfig(chatID, userID))
	return err
}

func banUserConfig(chatID int64, userID int64) tgbotapi.BanChatMemberConfig {
	return tgbotapi.BanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: chatID,
			UserID: userID,
		},
		RevokeMessages: true,
	}
}

func (client botAPIClient) SendMessage(chatID int64, text string) error {
	message := tgbotapi.NewMessage(chatID, text)
	_, err := client.bot.Send(message)
	return err
}

func (client botAPIClient) FetchAccountEvidence(chatID int64, user tgbotapi.User) AccountEvidence {
	evidence := AccountEvidenceFromUser(user)
	member, err := client.bot.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: user.ID,
		},
	})
	if err != nil {
		evidence.ChatMemberLookupError = err.Error()
		return evidence
	}
	ApplyChatMemberEvidence(&evidence, member)
	return evidence
}

func AccountEvidenceFromUser(user tgbotapi.User) AccountEvidence {
	return AccountEvidence{
		ID:                     user.ID,
		FirstName:              user.FirstName,
		LastName:               user.LastName,
		Username:               user.UserName,
		LanguageCode:           user.LanguageCode,
		IsBot:                  user.IsBot,
		BotAPIAccountCreatedAt: "unavailable: Telegram Bot API does not expose user account creation time",
		BotAPIDescription:      "unavailable: Telegram Bot API does not expose user profile description/bio for group members",
	}
}

func ApplyChatMemberEvidence(evidence *AccountEvidence, member tgbotapi.ChatMember) {
	evidence.ChatMemberStatus = member.Status
	evidence.ChatMemberCustomTitle = member.CustomTitle
	evidence.ChatMemberUntilDate = member.UntilDate
	evidence.ChatMemberCanDelete = member.CanDeleteMessages
	evidence.ChatMemberCanRestrict = member.CanRestrictMembers
	evidence.ChatMemberCanManageChat = member.CanManageChat
	evidence.ChatMemberCanPromote = member.CanPromoteMembers
	raw, err := json.Marshal(member)
	if err == nil {
		evidence.ChatMemberRawJSON = string(raw)
	}
}

func BanNotice(user tgbotapi.User, reason string, deletedCount int) string {
	username := "-"
	if user.UserName != "" {
		username = "@" + user.UserName
	}
	return fmt.Sprintf("%d / %s / %s is banned because %s, %d messages deleted", user.ID, fullName(user), username, reason, deletedCount)
}

func fullName(user tgbotapi.User) string {
	name := strings.TrimSpace(strings.TrimSpace(user.FirstName) + " " + strings.TrimSpace(user.LastName))
	if name == "" {
		return "-"
	}
	return name
}

func sampleText(text string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes])
}
