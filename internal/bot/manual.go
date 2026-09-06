package bot

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type ManualModerationRequest struct {
	ChatID            int64
	ChatType          string
	ChatTitle         string
	ChatUsername      string
	UserID            int64
	MessageIDs        []int
	Reason            string
	MessageTextSample string
	Now               time.Time
}

type ManualModerationSummary struct {
	ChatID        int64
	UserID        int64
	Deleted       int
	Banned        bool
	AlreadyBanned bool
	Errors        int
}

func (summary ManualModerationSummary) Summary() string {
	return fmt.Sprintf(
		"manual_moderation chat=%d user=%d deleted=%d banned=%t already_banned=%t errors=%d",
		summary.ChatID,
		summary.UserID,
		summary.Deleted,
		summary.Banned,
		summary.AlreadyBanned,
		summary.Errors,
	)
}

func ManualModerate(cfg Config, store *FileStore, client TelegramClient, req ManualModerationRequest) (ManualModerationSummary, error) {
	summary := ManualModerationSummary{ChatID: req.ChatID, UserID: req.UserID}
	if req.ChatID == 0 {
		return summary, fmt.Errorf("chat ID is required")
	}
	if req.UserID == 0 {
		return summary, fmt.Errorf("user ID is required")
	}
	if !cfg.AllowsChat(req.ChatID) {
		return summary, fmt.Errorf("chat %d is not allowed by config", req.ChatID)
	}
	messageIDs := normalizeManualMessageIDs(req.MessageIDs)
	if len(messageIDs) == 0 {
		return summary, fmt.Errorf("at least one message ID is required")
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "manual"
	}

	history := store.History(req.ChatID, req.UserID)
	evidence := client.FetchAccountEvidence(req.ChatID, tgbotapi.User{ID: req.UserID})
	evidence.ObservedFirstSeenAt = history.FirstSeenAt
	evidence.ObservedJoinedAt = history.JoinedAt
	if evidence.ChatMemberLookupError != "" {
		return summary, fmt.Errorf("verify chat member: %s", evidence.ChatMemberLookupError)
	}
	if protectedChatMemberStatus(evidence.ChatMemberStatus) {
		return summary, fmt.Errorf("refusing to moderate protected chat member status=%s", evidence.ChatMemberStatus)
	}

	action := ModerationAction{
		At:                         now,
		ChatID:                     req.ChatID,
		ChatType:                   req.ChatType,
		ChatTitle:                  req.ChatTitle,
		ChatUsername:               req.ChatUsername,
		UserID:                     req.UserID,
		User:                       evidence,
		MessageID:                  messageIDs[0],
		MessageDate:                now,
		MessageTextSample:          sampleText(req.MessageTextSample, 240),
		ObservedMessageCountBefore: history.MessageCount,
		ObservedFirstSeenAt:        history.FirstSeenAt,
		ObservedJoinedAt:           history.JoinedAt,
		RecentMessageIDs:           append([]int(nil), history.RecentMessageIDs...),
		Reason:                     reason,
		DeletedMessageIDs:          append([]int(nil), messageIDs...),
		DryRun:                     cfg.DryRun,
		TelegramLimitations: []string{
			"Telegram Bot API does not expose account creation date",
			"Telegram Bot API does not expose user profile description/bio for group members",
			"Telegram Bot API does not expose the client-side Report Spam action",
		},
	}

	log.Printf(
		"manual moderating chat=%d user=%d username=%q name=%q reason=%s delete_messages=%v dry_run=%t",
		req.ChatID,
		req.UserID,
		evidence.Username,
		strings.TrimSpace(strings.TrimSpace(evidence.FirstName)+" "+strings.TrimSpace(evidence.LastName)),
		reason,
		messageIDs,
		cfg.DryRun,
	)

	if err := store.MarkManualModerated(req.ChatID, req.UserID, messageIDs, now, reason, cfg.DeleteRecentLimit); err != nil {
		action.Errors = append(action.Errors, "record manual moderation: "+err.Error())
		_ = store.AppendModerationAction(action, cfg.ActionLogLimit)
		return summary, err
	}
	if cfg.DryRun {
		if err := store.AppendModerationAction(action, cfg.ActionLogLimit); err != nil {
			return summary, err
		}
		return summary, nil
	}

	for _, messageID := range messageIDs {
		if err := client.DeleteMessage(req.ChatID, messageID); err != nil {
			action.Errors = append(action.Errors, fmt.Sprintf("delete message %d: %v", messageID, err))
			log.Printf("manual delete message failed chat=%d user=%d message=%d: %v", req.ChatID, req.UserID, messageID, err)
			continue
		}
		summary.Deleted++
	}
	action.DeletedCount = summary.Deleted

	if evidence.ChatMemberStatus == "kicked" {
		action.Banned = true
		summary.Banned = true
		summary.AlreadyBanned = true
	} else if err := client.BanUser(req.ChatID, req.UserID); err != nil {
		action.Errors = append(action.Errors, "ban user: "+err.Error())
		_ = store.AppendModerationAction(action, cfg.ActionLogLimit)
		summary.Errors = len(action.Errors)
		return summary, fmt.Errorf("ban user chat=%d user=%d: %w", req.ChatID, req.UserID, err)
	} else {
		action.Banned = true
		summary.Banned = true
	}

	notice := BanNoticeFromEvidence(evidence, reason, summary.Deleted)
	if err := client.SendMessage(cfg.NoticeChatID(req.ChatID), notice); err != nil {
		action.Errors = append(action.Errors, "send ban notice: "+err.Error())
		log.Printf("manual send ban notice failed chat=%d user=%d: %v", req.ChatID, req.UserID, err)
	}

	summary.Errors = len(action.Errors)
	if err := store.AppendModerationAction(action, cfg.ActionLogLimit); err != nil {
		return summary, err
	}
	if len(action.Errors) > 0 {
		return summary, fmt.Errorf("manual moderation completed with %d error(s)", len(action.Errors))
	}
	return summary, nil
}

func normalizeManualMessageIDs(raw []int) []int {
	seen := make(map[int]bool, len(raw))
	ids := make([]int, 0, len(raw))
	for _, id := range raw {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}
