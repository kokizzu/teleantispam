package bot

import (
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type fakeManualTelegramClient struct {
	evidence AccountEvidence
	deleted  []int
	banned   []int64
	sent     []string
}

func (client *fakeManualTelegramClient) DeleteMessage(_ int64, messageID int) error {
	client.deleted = append(client.deleted, messageID)
	return nil
}

func (client *fakeManualTelegramClient) BanUser(_ int64, userID int64) error {
	client.banned = append(client.banned, userID)
	return nil
}

func (client *fakeManualTelegramClient) SendMessage(_ int64, text string) error {
	client.sent = append(client.sent, text)
	return nil
}

func (client *fakeManualTelegramClient) FetchAccountEvidence(_ int64, user tgbotapi.User) AccountEvidence {
	evidence := client.evidence
	if evidence.ID == 0 {
		evidence.ID = user.ID
	}
	return evidence
}

func TestManualModerateDeletesBansLogsAndMarksHistory(t *testing.T) {
	now := time.Unix(1000, 0)
	chatID := int64(-1001116539442)
	userID := int64(1028520138)
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.RecordMessage(chatID, userID, 69692, now.Add(-time.Minute), 10); err != nil {
		t.Fatalf("record message: %v", err)
	}

	client := &fakeManualTelegramClient{
		evidence: AccountEvidence{
			ID:               userID,
			FirstName:        "Shavon",
			LastName:         "Ginz",
			Username:         "spam_user",
			ChatMemberStatus: "member",
		},
	}
	summary, err := ManualModerate(testConfig(), store, client, ManualModerationRequest{
		ChatID:            chatID,
		UserID:            userID,
		MessageIDs:        []int{69693, 69692, 69692},
		Reason:            "cjk-heavy",
		MessageTextSample: "manual linked CJK spam",
		Now:               now,
	})
	if err != nil {
		t.Fatalf("manual moderate: %v", err)
	}
	if summary.Deleted != 2 || !summary.Banned || summary.Errors != 0 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if !reflect.DeepEqual(client.deleted, []int{69692, 69693}) {
		t.Fatalf("unexpected deleted messages: %#v", client.deleted)
	}
	if !reflect.DeepEqual(client.banned, []int64{userID}) {
		t.Fatalf("unexpected bans: %#v", client.banned)
	}
	if len(client.sent) != 1 || !strings.Contains(client.sent[0], "1028520138 / Shavon Ginz / @spam_user is banned because cjk-heavy, 2 messages deleted") {
		t.Fatalf("unexpected notice: %#v", client.sent)
	}

	actions := store.ModerationActions()
	if len(actions) != 1 {
		t.Fatalf("unexpected action count: %d", len(actions))
	}
	action := actions[0]
	if action.Reason != "cjk-heavy" || action.DeletedCount != 2 || !action.Banned {
		t.Fatalf("unexpected action: %#v", action)
	}

	history := store.state.Chats[strconv.FormatInt(chatID, 10)][strconv.FormatInt(userID, 10)]
	if history.MessageCount != 2 || history.LastModeration != "cjk-heavy" || history.LastModeratedAt.IsZero() {
		t.Fatalf("unexpected history: %#v", history)
	}
	if !reflect.DeepEqual(history.RecentMessageIDs, []int{69692, 69693}) {
		t.Fatalf("unexpected history message IDs: %#v", history.RecentMessageIDs)
	}
}

func TestManualModerateRefusesProtectedMember(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	client := &fakeManualTelegramClient{
		evidence: AccountEvidence{
			ID:               1,
			ChatMemberStatus: "administrator",
		},
	}

	_, err = ManualModerate(testConfig(), store, client, ManualModerationRequest{
		ChatID:     -100,
		UserID:     1,
		MessageIDs: []int{2},
		Reason:     "manual",
		Now:        time.Unix(1000, 0),
	})
	if err == nil || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("expected protected member error, got %v", err)
	}
	if len(client.deleted) != 0 || len(client.banned) != 0 || len(store.ModerationActions()) != 0 {
		t.Fatalf("protected member should not be changed")
	}
}

func TestManualModerateReturnsDeleteErrorsAfterLoggingBan(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	client := &fakeManualDeleteErrorClient{
		fakeManualTelegramClient: fakeManualTelegramClient{
			evidence: AccountEvidence{ID: 1, ChatMemberStatus: "member"},
		},
	}

	summary, err := ManualModerate(testConfig(), store, client, ManualModerationRequest{
		ChatID:     -100,
		UserID:     1,
		MessageIDs: []int{2},
		Reason:     "manual",
		Now:        time.Unix(1000, 0),
	})
	if err == nil {
		t.Fatalf("expected delete error")
	}
	if summary.Deleted != 0 || !summary.Banned || summary.Errors != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	actions := store.ModerationActions()
	if len(actions) != 1 || len(actions[0].Errors) != 1 || !actions[0].Banned {
		t.Fatalf("unexpected action log: %#v", actions)
	}
}

type fakeManualDeleteErrorClient struct {
	fakeManualTelegramClient
}

func (client *fakeManualDeleteErrorClient) DeleteMessage(_ int64, messageID int) error {
	client.deleted = append(client.deleted, messageID)
	return errors.New("delete failed")
}

func TestApplyChatMemberEvidenceFillsMissingUserFields(t *testing.T) {
	evidence := AccountEvidence{}
	ApplyChatMemberEvidence(&evidence, tgbotapi.ChatMember{
		User: &tgbotapi.User{
			ID:        7785227080,
			FirstName: "Nicole",
			LastName:  "Soo",
			UserName:  "Nicoleeeso",
		},
		Status: "member",
	})

	if evidence.ID != 7785227080 ||
		evidence.FirstName != "Nicole" ||
		evidence.LastName != "Soo" ||
		evidence.Username != "Nicoleeeso" ||
		evidence.ChatMemberStatus != "member" {
		t.Fatalf("unexpected evidence: %#v", evidence)
	}
}
