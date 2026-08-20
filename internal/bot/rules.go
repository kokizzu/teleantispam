package bot

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

var cryptoSpamPattern = regexp.MustCompile(`(?i)\b(airdrop|bitcoin|btc|crypto|defi|ethereum|eth|investment|pump|signal|ton|trading|usdt|wallet|web3)\b`)

type MessageEvent struct {
	ChatID    int64
	UserID    int64
	MessageID int
	Text      string
	At        time.Time
	Now       time.Time
}

type HistoryView struct {
	MessageCount     int
	FirstSeenAt      time.Time
	JoinedAt         time.Time
	RecentMessageIDs []int
}

type ModerationDecision struct {
	Moderate         bool
	Reason           string
	DeleteMessageIDs []int
	BanUser          bool
}

func EvaluateMessage(cfg Config, history HistoryView, event MessageEvent) ModerationDecision {
	if history.MessageCount >= cfg.MaxSafePosts {
		return ModerationDecision{}
	}
	if tooOldForModeration(cfg, event) {
		return ModerationDecision{}
	}

	reason := suspiciousReason(event.Text)
	if reason == "" {
		return ModerationDecision{}
	}

	if !eligibleLowHistory(cfg, history, event.At) {
		return ModerationDecision{}
	}

	return ModerationDecision{
		Moderate:         true,
		Reason:           reason,
		DeleteMessageIDs: messageIDsToDelete(history.RecentMessageIDs, event.MessageID, cfg.DeleteRecentLimit),
		BanUser:          true,
	}
}

func tooOldForModeration(cfg Config, event MessageEvent) bool {
	if event.Now.IsZero() || event.At.IsZero() {
		return false
	}
	return event.At.Before(event.Now.Add(-cfg.MaxMessageAge))
}

func eligibleLowHistory(cfg Config, history HistoryView, now time.Time) bool {
	if !history.JoinedAt.IsZero() && !now.Before(history.JoinedAt) && now.Sub(history.JoinedAt) <= cfg.JoinWindow {
		return true
	}
	if cfg.AllowUnknownNoHistory && history.MessageCount <= cfg.LowHistoryPosts {
		return true
	}
	return false
}

func suspiciousReason(text string) string {
	normalized := strings.TrimSpace(strings.ToLower(text))
	if normalized == "" {
		return ""
	}
	if normalized == "0" {
		return "zero-message"
	}
	if cryptoSpamPattern.MatchString(normalized) {
		return "crypto-keyword"
	}
	if cjkHeavy(text) {
		return "cjk-heavy"
	}
	if cyrillicHeavy(text) {
		return "cyrillic-heavy"
	}
	return ""
}

func cjkHeavy(text string) bool {
	han := 0
	latin := 0
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.Is(unicode.Latin, r):
			latin++
		}
	}
	return han >= 2 && latin <= 4
}

func cyrillicHeavy(text string) bool {
	cyrillic := 0
	latin := 0
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		case unicode.Is(unicode.Latin, r):
			latin++
		}
	}
	return cyrillic >= 2 && latin <= 4
}

func messageIDsToDelete(previous []int, current int, limit int) []int {
	if limit < 1 {
		limit = 1
	}
	seen := make(map[int]bool, len(previous)+1)
	ids := make([]int, 0, len(previous)+1)
	for _, id := range previous {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if current > 0 && !seen[current] {
		ids = append(ids, current)
	}
	sort.Ints(ids)
	if len(ids) > limit {
		ids = ids[len(ids)-limit:]
	}
	return ids
}
