package bot

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

var cryptoSpamPattern = regexp.MustCompile(`(?i)\b(airdrop|bitcoin|btc|crypto|defi|ethereum|eth|investment|pump|signal|ton|trading|usdt|wallet|web3)\b`)
var unknownNoHistoryContactPattern = regexp.MustCompile(`(?i)(\bt\.me/[a-z0-9_]+|\btelegram\.me/[a-z0-9_]+|@[a-z0-9_]{4,})`)
var unknownNoHistoryCryptoPitchPattern = regexp.MustCompile(`(?i)\b(airdrop|earned?|earning|income|investment|mentor(ship)?|profit|pump|signal|trading|usdt|wallet)\b`)
var unknownNoHistoryCyrillicPitchPattern = regexp.MustCompile(`(?i)(график|доход|занятост|заработ|команд|набор|онлайн|подработ|пишите|работ|ставьте|удален)`)
var unknownNoHistoryCJKPitchPattern = regexp.MustCompile(`(送彩金|加入群|进群|联系.{0,4}(客服|我)|客服|兼职|赚钱|收益|投资|理财|钱包|空投|交易|博彩|彩票|下注|代理|私信|扫码|领取|福利|优惠|返利|会员|推广|网赌|投注)`)
var forwardedFinanceTermPattern = regexp.MustCompile(`(?i)\b(airdrop|bitcoin|btc|crypto|defi|ethereum|eth|finance|financial|forex|fx|income|invest(?:ing|ment)?|market|profit|pump|signal|stock|trading|usdt|wallet|web3)\b`)
var forwardedFinancePitchPattern = regexp.MustCompile(`(?i)(\bt\.me/[a-z0-9_]+|\btelegram\.me/[a-z0-9_]+|@[a-z0-9_]{4,}|\b(channel|earned?|earning|group|income|invest(?:ing|ment)?|join|mentor(ship)?|profit|pump|signal|subscribe|trading|usdt|vip|wallet)\b)`)
var financeCommercialTermPattern = regexp.MustCompile(`(?i)\b(finance|financial|forex|income|invest(?:ing|ment)?|market|profit|stock|trading|wealth)\b`)
var privateTelegramInvitePattern = regexp.MustCompile(`(?i)(?:^|[\s(])(?:https?://)?(?:t\.me|telegram\.me)/(?:\+[a-z0-9_-]+|joinchat/[a-z0-9_-]+)\b`)

type MessageEvent struct {
	ChatID    int64
	UserID    int64
	MessageID int
	Text      string
	At        time.Time
	Now       time.Time

	ForwardedFromChat         bool
	ForwardedFromChatTitle    string
	ForwardedFromChatUsername string
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

	reason := suspiciousReason(event)
	if reason == "" {
		return ModerationDecision{}
	}

	if !eligibleLowHistory(cfg, history, event, reason) {
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

func eligibleLowHistory(cfg Config, history HistoryView, event MessageEvent, reason string) bool {
	now := event.At
	if !history.JoinedAt.IsZero() && !now.Before(history.JoinedAt) && now.Sub(history.JoinedAt) <= cfg.JoinWindow {
		return true
	}
	if history.MessageCount > cfg.LowHistoryPosts {
		return false
	}
	if cfg.AllowUnknownNoHistory {
		return true
	}
	return highConfidenceUnknownNoHistorySpam(reason, event)
}

func highConfidenceUnknownNoHistorySpam(reason string, event MessageEvent) bool {
	normalized := strings.TrimSpace(strings.ToLower(event.Text))
	if normalized == "" {
		return reason == "forwarded-finance-group" && forwardedFinanceGroupSpam(event)
	}
	switch reason {
	case "finance-private-invite":
		return financePrivateInviteSpam(event.Text)
	case "forwarded-finance-group":
		return forwardedFinanceGroupSpam(event)
	case "cjk-heavy":
		return unknownNoHistoryCJKPitchPattern.MatchString(normalized)
	case "crypto-keyword":
		if !unknownNoHistoryContactPattern.MatchString(normalized) {
			return false
		}
		return unknownNoHistoryCryptoPitchPattern.MatchString(normalized)
	case "cyrillic-heavy":
		if !unknownNoHistoryContactPattern.MatchString(normalized) {
			return false
		}
		return unknownNoHistoryCyrillicPitchPattern.MatchString(normalized)
	default:
		return false
	}
}

func suspiciousReason(event MessageEvent) string {
	if forwardedFinanceGroupSpam(event) {
		return "forwarded-finance-group"
	}

	normalized := strings.TrimSpace(strings.ToLower(event.Text))
	if normalized == "" {
		return ""
	}
	if normalized == "0" {
		return "zero-message"
	}
	if financePrivateInviteSpam(normalized) {
		return "finance-private-invite"
	}
	if cryptoSpamPattern.MatchString(normalized) {
		return "crypto-keyword"
	}
	if cjkHeavy(event.Text) {
		return "cjk-heavy"
	}
	if cyrillicHeavy(event.Text) {
		return "cyrillic-heavy"
	}
	return ""
}

func financePrivateInviteSpam(text string) bool {
	return financeCommercialTermPattern.MatchString(text) &&
		privateTelegramInvitePattern.MatchString(text)
}

func forwardedFinanceGroupSpam(event MessageEvent) bool {
	if !event.ForwardedFromChat {
		return false
	}
	combined := strings.TrimSpace(strings.Join([]string{
		event.Text,
		event.ForwardedFromChatTitle,
		event.ForwardedFromChatUsername,
	}, " "))
	if combined == "" {
		return false
	}
	return forwardedFinanceTermPattern.MatchString(combined) &&
		forwardedFinancePitchPattern.MatchString(combined)
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
	return han >= 2 && (latin <= 4 || han >= latin*3)
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
	return cyrillic >= 2 && (latin <= 4 || cyrillic >= latin*3)
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
