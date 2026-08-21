package bot

import (
	"fmt"
	"strings"
	"time"
)

const DefaultRetryFailedActionMaxAge = 48 * time.Hour

type RetryFailedModerationSummary struct {
	Candidates int
	Attempted  int
	Deleted    int
	Banned     int
	Failed     int
	Skipped    int
}

func RetryFailedModerationActions(cfg Config, store *FileStore, client TelegramClient, now time.Time, maxAge time.Duration) (RetryFailedModerationSummary, error) {
	if maxAge <= 0 {
		maxAge = DefaultRetryFailedActionMaxAge
	}

	actions := store.ModerationActions()
	candidates := make(map[string]ModerationAction)
	successes := make(map[string]time.Time)

	for _, action := range actions {
		key := retryActionKey(action)
		if action.Banned {
			successes[key] = action.At
			delete(candidates, key)
			continue
		}
		if !retryableModerationAction(action, now, maxAge) {
			continue
		}
		candidates[key] = action
	}

	summary := RetryFailedModerationSummary{Candidates: len(candidates)}
	for key, action := range candidates {
		if successAt, ok := successes[key]; ok && successAt.After(action.At) {
			summary.Skipped++
			continue
		}

		retry := action
		retry.At = now.UTC()
		retry.Retry = true
		retry.RetryOf = action.At
		retry.Errors = nil
		retry.DeletedCount = 0
		retry.Banned = false

		for _, messageID := range uniqueRetryMessageIDs(action) {
			if err := client.DeleteMessage(action.ChatID, messageID); err != nil {
				retry.Errors = append(retry.Errors, fmt.Sprintf("retry delete message %d: %v", messageID, err))
				continue
			}
			retry.DeletedCount++
			summary.Deleted++
		}

		if err := client.BanUser(action.ChatID, action.UserID); err != nil {
			retry.Errors = append(retry.Errors, "retry ban user: "+err.Error())
		} else {
			retry.Banned = true
			summary.Banned++
			notice := BanNoticeFromEvidence(action.User, action.Reason, retry.DeletedCount)
			if err := client.SendMessage(cfg.NoticeChatID(action.ChatID), notice); err != nil {
				retry.Errors = append(retry.Errors, "retry send ban notice: "+err.Error())
			}
		}

		summary.Attempted++
		if len(retry.Errors) > 0 {
			summary.Failed++
		}
		if err := store.AppendModerationAction(retry, cfg.ActionLogLimit); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

func (summary RetryFailedModerationSummary) Summary() string {
	return fmt.Sprintf(
		"retry_failed_moderation candidates=%d attempted=%d banned=%d deleted=%d failed=%d skipped=%d",
		summary.Candidates,
		summary.Attempted,
		summary.Banned,
		summary.Deleted,
		summary.Failed,
		summary.Skipped,
	)
}

func retryableModerationAction(action ModerationAction, now time.Time, maxAge time.Duration) bool {
	if action.DryRun || action.Banned || action.ChatID == 0 || action.UserID == 0 {
		return false
	}
	if !action.At.IsZero() && now.Sub(action.At) > maxAge {
		return false
	}
	for _, errText := range action.Errors {
		errText = strings.ToLower(errText)
		if strings.Contains(errText, "not enough rights") ||
			strings.Contains(errText, "message can't be deleted") ||
			strings.Contains(errText, "ban user:") {
			return true
		}
	}
	return false
}

func retryActionKey(action ModerationAction) string {
	return fmt.Sprintf("%d:%d", action.ChatID, action.UserID)
}

func uniqueRetryMessageIDs(action ModerationAction) []int {
	seen := make(map[int]bool)
	var ids []int
	for _, id := range append(append([]int(nil), action.DeletedMessageIDs...), action.MessageID) {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}
