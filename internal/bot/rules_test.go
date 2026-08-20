package bot

import (
	"reflect"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		MaxSafePosts:      10,
		LowHistoryPosts:   2,
		JoinWindow:        48 * time.Hour,
		MaxMessageAge:     24 * time.Hour,
		DeleteRecentLimit: 10,
		ActionLogLimit:    10000,
	}
}

func TestEvaluateMessageNeverModeratesObservedTenPostUser(t *testing.T) {
	now := time.Unix(1000, 0)
	decision := EvaluateMessage(testConfig(), HistoryView{
		MessageCount:     10,
		JoinedAt:         now.Add(-time.Minute),
		RecentMessageIDs: []int{1, 2, 3},
	}, MessageEvent{
		MessageID: 4,
		Text:      "免费 bitcoin крипто 0",
		At:        now,
		Now:       now,
	})

	if decision.Moderate {
		t.Fatalf("expected >=10 observed posts to be a hard allow guard, got %#v", decision)
	}
}

func TestEvaluateMessageModeratesNewZeroSender(t *testing.T) {
	now := time.Unix(1000, 0)
	decision := EvaluateMessage(testConfig(), HistoryView{
		MessageCount:     0,
		JoinedAt:         now.Add(-time.Minute),
		RecentMessageIDs: []int{11, 12},
	}, MessageEvent{
		MessageID: 13,
		Text:      "0",
		At:        now,
		Now:       now,
	})

	if !decision.Moderate || !decision.BanUser || decision.Reason != "zero-message" {
		t.Fatalf("expected zero-message moderation, got %#v", decision)
	}
	if !reflect.DeepEqual(decision.DeleteMessageIDs, []int{11, 12, 13}) {
		t.Fatalf("unexpected delete ids: %#v", decision.DeleteMessageIDs)
	}
}

func TestEvaluateMessageModeratesLowHistoryCJKText(t *testing.T) {
	now := time.Unix(1000, 0)
	decision := EvaluateMessage(testConfig(), HistoryView{
		MessageCount: 1,
		JoinedAt:     now.Add(-time.Hour),
	}, MessageEvent{
		MessageID: 8,
		Text:      "送彩金 加入群",
		At:        now,
		Now:       now,
	})

	if !decision.Moderate || decision.Reason != "cjk-heavy" {
		t.Fatalf("expected cjk-heavy moderation, got %#v", decision)
	}
}

func TestEvaluateMessageModeratesLowHistoryCyrillicText(t *testing.T) {
	now := time.Unix(1000, 0)
	decision := EvaluateMessage(testConfig(), HistoryView{
		MessageCount: 2,
	}, MessageEvent{
		MessageID: 9,
		Text:      "быстрый заработок",
		At:        now,
		Now:       now,
	})

	if !decision.Moderate || decision.Reason != "cyrillic-heavy" {
		t.Fatalf("expected cyrillic-heavy moderation, got %#v", decision)
	}
}

func TestEvaluateMessageModeratesCryptoTextFromNoHistoryUser(t *testing.T) {
	now := time.Unix(1000, 0)
	decision := EvaluateMessage(testConfig(), HistoryView{
		MessageCount: 0,
	}, MessageEvent{
		MessageID: 10,
		Text:      "join crypto airdrop wallet now",
		At:        now,
		Now:       now,
	})

	if !decision.Moderate || decision.Reason != "crypto-keyword" {
		t.Fatalf("expected crypto-keyword moderation, got %#v", decision)
	}
}

func TestEvaluateMessageAllowsOrdinaryLowHistoryText(t *testing.T) {
	now := time.Unix(1000, 0)
	decision := EvaluateMessage(testConfig(), HistoryView{
		MessageCount: 0,
	}, MessageEvent{
		MessageID: 10,
		Text:      "hi, I am learning Go and have a question about maps",
		At:        now,
		Now:       now,
	})

	if decision.Moderate {
		t.Fatalf("expected ordinary text to be allowed, got %#v", decision)
	}
}

func TestEvaluateMessageAllowsOldPendingStartupMessage(t *testing.T) {
	now := time.Unix(1000, 0)
	decision := EvaluateMessage(testConfig(), HistoryView{
		MessageCount: 0,
	}, MessageEvent{
		MessageID: 10,
		Text:      "0",
		At:        now.Add(-25 * time.Hour),
		Now:       now,
	})

	if decision.Moderate {
		t.Fatalf("expected message older than startup max age to be allowed, got %#v", decision)
	}
}

func TestEvaluateMessageUsesDeleteLimit(t *testing.T) {
	now := time.Unix(1000, 0)
	decision := EvaluateMessage(Config{
		MaxSafePosts:      10,
		LowHistoryPosts:   2,
		JoinWindow:        48 * time.Hour,
		MaxMessageAge:     24 * time.Hour,
		DeleteRecentLimit: 3,
	}, HistoryView{
		MessageCount:     1,
		RecentMessageIDs: []int{1, 2, 3, 4},
	}, MessageEvent{
		MessageID: 5,
		Text:      "0",
		At:        now,
		Now:       now,
	})

	if !reflect.DeepEqual(decision.DeleteMessageIDs, []int{3, 4, 5}) {
		t.Fatalf("unexpected limited delete ids: %#v", decision.DeleteMessageIDs)
	}
}
