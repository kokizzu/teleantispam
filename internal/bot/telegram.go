package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
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

type MessageForwardOrigin struct {
	Type            string         `json:"type"`
	Date            int            `json:"date"`
	SenderUser      *tgbotapi.User `json:"sender_user,omitempty"`
	SenderUserName  string         `json:"sender_user_name,omitempty"`
	SenderChat      *tgbotapi.Chat `json:"sender_chat,omitempty"`
	AuthorSignature string         `json:"author_signature,omitempty"`
	Chat            *tgbotapi.Chat `json:"chat,omitempty"`
	MessageID       int            `json:"message_id,omitempty"`
}

type MessageForwardMetadata struct {
	ForwardedFromChat         bool
	ForwardedFromChatTitle    string
	ForwardedFromChatUsername string
}

type rawTelegramMessage struct {
	tgbotapi.Message
	ForwardOrigin *MessageForwardOrigin `json:"forward_origin,omitempty"`
}

type rawTelegramUpdate struct {
	UpdateID int                 `json:"update_id"`
	Message  *rawTelegramMessage `json:"message,omitempty"`
}

type rawGetUpdatesResponse struct {
	OK          bool                `json:"ok"`
	Result      []rawTelegramUpdate `json:"result"`
	ErrorCode   int                 `json:"error_code,omitempty"`
	Description string              `json:"description,omitempty"`
}

func NewTelegramClient(token string) (TelegramClient, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return botAPIClient{bot: api}, nil
}

func Run(ctx context.Context, cfg Config, store *FileStore) error {
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return err
	}
	log.Printf("authorized TeleAntiSpam2Bot as @%s", api.Self.UserName)

	client := botAPIClient{bot: api}
	nextOffset, err := drainPendingUpdates(ctx, cfg, store, client)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		updates, err := fetchRawUpdates(ctx, cfg.Token, nextOffset, 100, cfg.PollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("%v", err)
			log.Printf("Failed to get updates, retrying in 3 seconds...")
			if !sleepWithContext(ctx, 3*time.Second) {
				return ctx.Err()
			}
			continue
		}
		nextOffset, err = HandleRawUpdates(cfg, store, client, updates, time.Now())
		if err != nil {
			log.Printf("handle update failed: %v", err)
		}
	}
}

func drainPendingUpdates(ctx context.Context, cfg Config, store *FileStore, client TelegramClient) (int, error) {
	nextOffset := 0
	for {
		select {
		case <-ctx.Done():
			return nextOffset, ctx.Err()
		default:
		}

		updates, err := fetchRawUpdates(ctx, cfg.Token, nextOffset, 100, 0)
		if err != nil {
			return nextOffset, fmt.Errorf("startup pending update scan: %w", err)
		}
		if len(updates) == 0 {
			return nextOffset, nil
		}
		nextOffset, err = HandleRawUpdates(cfg, store, client, updates, time.Now())
		if err != nil {
			return nextOffset, err
		}
		if len(updates) < 100 {
			return nextOffset, nil
		}
	}
}

func fetchRawUpdates(ctx context.Context, token string, offset int, limit int, timeout int) ([]rawTelegramUpdate, error) {
	values := url.Values{}
	if offset > 0 {
		values.Set("offset", strconv.Itoa(offset))
	}
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	if timeout > 0 {
		values.Set("timeout", strconv.Itoa(timeout))
	}
	allowedUpdates, err := json.Marshal([]string{"message"})
	if err != nil {
		return nil, err
	}
	values.Set("allowed_updates", string(allowedUpdates))

	clientTimeout := time.Duration(timeout+10) * time.Second
	if clientTimeout < 10*time.Second {
		clientTimeout = 10 * time.Second
	}
	httpClient := http.Client{Timeout: clientTimeout}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://api.telegram.org/bot"+token+"/getUpdates",
		strings.NewReader(values.Encode()),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, telegramRequestError(err)
	}
	defer resp.Body.Close()

	var decoded rawGetUpdatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if !decoded.OK {
		return nil, fmt.Errorf("getUpdates failed: code=%d description=%s", decoded.ErrorCode, decoded.Description)
	}
	return decoded.Result, nil
}

func telegramRequestError(err error) error {
	var requestError *url.Error
	if errors.As(err, &requestError) {
		return fmt.Errorf("Telegram getUpdates request failed: %w", requestError.Err)
	}
	return fmt.Errorf("Telegram getUpdates request failed")
}

func sleepWithContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func HandleRawUpdates(cfg Config, store *FileStore, client TelegramClient, updates []rawTelegramUpdate, now time.Time) (int, error) {
	nextOffset := 0
	for _, update := range updates {
		if update.UpdateID >= nextOffset {
			nextOffset = update.UpdateID + 1
		}
		if err := HandleRawUpdate(cfg, store, client, update, now); err != nil {
			return nextOffset, err
		}
	}
	return nextOffset, nil
}

func HandleRawUpdate(cfg Config, store *FileStore, client TelegramClient, update rawTelegramUpdate, now time.Time) error {
	if update.Message == nil {
		return nil
	}
	return HandleTelegramMessage(cfg, store, client, &update.Message.Message, forwardMetadataFromRawMessage(update.Message), now)
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
	return HandleTelegramMessage(cfg, store, client, message, forwardMetadataFromMessage(message), now)
}

func HandleTelegramMessage(cfg Config, store *FileStore, client TelegramClient, message *tgbotapi.Message, forward MessageForwardMetadata, now time.Time) error {
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

		ForwardedFromChat:         forward.ForwardedFromChat,
		ForwardedFromChatTitle:    forward.ForwardedFromChatTitle,
		ForwardedFromChatUsername: forward.ForwardedFromChatUsername,
	})

	if !decision.Moderate {
		return store.RecordMessage(chatID, userID, message.MessageID, messageTime, cfg.DeleteRecentLimit)
	}

	evidence := client.FetchAccountEvidence(chatID, *message.From)
	evidence.ObservedFirstSeenAt = history.FirstSeenAt
	evidence.ObservedJoinedAt = history.JoinedAt

	if evidence.ChatMemberLookupError != "" {
		log.Printf(
			"skipping unverified chat member chat=%d user=%d lookup_error=%q reason=%s",
			chatID,
			userID,
			evidence.ChatMemberLookupError,
			decision.Reason,
		)
		return store.RecordMessage(chatID, userID, message.MessageID, messageTime, cfg.DeleteRecentLimit)
	}

	if protectedChatMemberStatus(evidence.ChatMemberStatus) {
		log.Printf(
			"skipping protected chat member chat=%d user=%d status=%s reason=%s",
			chatID,
			userID,
			evidence.ChatMemberStatus,
			decision.Reason,
		)
		return store.RecordMessage(chatID, userID, message.MessageID, messageTime, cfg.DeleteRecentLimit)
	}

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

func forwardMetadataFromRawMessage(message *rawTelegramMessage) MessageForwardMetadata {
	metadata := forwardMetadataFromMessage(&message.Message)
	if message.ForwardOrigin == nil {
		return metadata
	}
	switch message.ForwardOrigin.Type {
	case "chat":
		metadata = forwardMetadataFromChat(message.ForwardOrigin.SenderChat)
	case "channel":
		metadata = forwardMetadataFromChat(message.ForwardOrigin.Chat)
	}
	return metadata
}

func forwardMetadataFromMessage(message *tgbotapi.Message) MessageForwardMetadata {
	if message == nil || message.ForwardFromChat == nil {
		return MessageForwardMetadata{}
	}
	return forwardMetadataFromChat(message.ForwardFromChat)
}

func forwardMetadataFromChat(chat *tgbotapi.Chat) MessageForwardMetadata {
	if chat == nil {
		return MessageForwardMetadata{}
	}
	return MessageForwardMetadata{
		ForwardedFromChat:         true,
		ForwardedFromChatTitle:    chat.Title,
		ForwardedFromChatUsername: chat.UserName,
	}
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
	if member.User != nil {
		if evidence.ID == 0 {
			evidence.ID = member.User.ID
		}
		if evidence.FirstName == "" {
			evidence.FirstName = member.User.FirstName
		}
		if evidence.LastName == "" {
			evidence.LastName = member.User.LastName
		}
		if evidence.Username == "" {
			evidence.Username = member.User.UserName
		}
		if evidence.LanguageCode == "" {
			evidence.LanguageCode = member.User.LanguageCode
		}
		evidence.IsBot = member.User.IsBot
	}
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
	return banNotice(user.ID, fullName(user), user.UserName, reason, deletedCount)
}

func BanNoticeFromEvidence(user AccountEvidence, reason string, deletedCount int) string {
	name := strings.TrimSpace(strings.TrimSpace(user.FirstName) + " " + strings.TrimSpace(user.LastName))
	if name == "" {
		name = "-"
	}
	return banNotice(user.ID, name, user.Username, reason, deletedCount)
}

func banNotice(userID int64, name string, rawUsername string, reason string, deletedCount int) string {
	username := "-"
	if rawUsername != "" {
		username = "@" + rawUsername
	}
	return fmt.Sprintf("%d / %s / %s is banned because %s, %d messages deleted", userID, name, username, reason, deletedCount)
}

func protectedChatMemberStatus(status string) bool {
	switch status {
	case "creator", "administrator":
		return true
	default:
		return false
	}
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
