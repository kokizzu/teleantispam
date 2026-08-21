package bot

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestRetryFailedModerationActionsRetriesLatestFailurePerUser(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.ActionLogLimit = 100
	now := time.Unix(2000, 0)

	oldFailure := ModerationAction{
		At:                now.Add(-time.Hour),
		ChatID:            -100123,
		UserID:            55,
		User:              AccountEvidence{ID: 55, FirstName: "Jeana", Username: "spammy"},
		MessageID:         102,
		Reason:            "zero-message",
		DeletedMessageIDs: []int{100, 101, 102},
		Errors:            []string{"ban user: Bad Request: not enough rights to restrict/unrestrict chat member"},
	}
	latestFailure := oldFailure
	latestFailure.At = now.Add(-30 * time.Minute)
	latestFailure.DeletedMessageIDs = []int{103, 104}
	latestFailure.MessageID = 104

	if err := store.AppendModerationAction(oldFailure, cfg.ActionLogLimit); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendModerationAction(latestFailure, cfg.ActionLogLimit); err != nil {
		t.Fatal(err)
	}

	client := &fakeTelegramClient{}
	summary, err := RetryFailedModerationActions(cfg, store, client, now, 48*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Candidates != 1 || summary.Attempted != 1 || summary.Banned != 1 || summary.Deleted != 2 || summary.Failed != 0 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if !reflect.DeepEqual(client.deleted, []int{103, 104}) {
		t.Fatalf("deleted = %#v", client.deleted)
	}
	if !reflect.DeepEqual(client.banned, []int64{55}) {
		t.Fatalf("banned = %#v", client.banned)
	}
	if len(client.sent) != 1 || client.sent[0].chatID != -100123 {
		t.Fatalf("sent = %#v", client.sent)
	}

	actions := store.ModerationActions()
	retry := actions[len(actions)-1]
	if !retry.Retry || retry.RetryOf != latestFailure.At || !retry.Banned || retry.DeletedCount != 2 {
		t.Fatalf("unexpected retry action: %#v", retry)
	}
}

func TestRetryFailedModerationActionsSkipsNoticeForAlreadyBannedUser(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	now := time.Unix(2000, 0)
	if err := store.AppendModerationAction(ModerationAction{
		At:                now.Add(-time.Hour),
		ChatID:            -100123,
		UserID:            55,
		User:              AccountEvidence{ID: 55, FirstName: "Jeana", Username: "spammy"},
		MessageID:         102,
		Reason:            "zero-message",
		DeletedMessageIDs: []int{100, 102},
		Errors:            []string{"ban user: Bad Request: not enough rights to restrict/unrestrict chat member"},
	}, cfg.ActionLogLimit); err != nil {
		t.Fatal(err)
	}

	client := &fakeTelegramClient{memberStatus: "kicked"}
	summary, err := RetryFailedModerationActions(cfg, store, client, now, 48*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Attempted != 1 || summary.AlreadyBanned != 1 || summary.Banned != 0 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if len(client.deleted) != 0 || len(client.banned) != 0 || len(client.sent) != 0 {
		t.Fatalf("already-banned retry should not repeat actions: %#v", client)
	}

	actions := store.ModerationActions()
	retry := actions[len(actions)-1]
	if !retry.Retry || !retry.Banned || retry.User.ChatMemberStatus != "kicked" {
		t.Fatalf("unexpected retry action: %#v", retry)
	}
}

func TestRetryFailedModerationActionsSkipsOldFailure(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	now := time.Unix(2000, 0)
	if err := store.AppendModerationAction(ModerationAction{
		At:                now.Add(-72 * time.Hour),
		ChatID:            -100123,
		UserID:            55,
		MessageID:         102,
		Reason:            "zero-message",
		DeletedMessageIDs: []int{102},
		Errors:            []string{"ban user: Bad Request: not enough rights to restrict/unrestrict chat member"},
	}, cfg.ActionLogLimit); err != nil {
		t.Fatal(err)
	}

	client := &fakeTelegramClient{}
	summary, err := RetryFailedModerationActions(cfg, store, client, now, 48*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Candidates != 0 || len(client.banned) != 0 || len(client.deleted) != 0 {
		t.Fatalf("unexpected retry: summary=%#v client=%#v", summary, client)
	}
}

func TestRetryFailedModerationActionsLogsRetryErrors(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	now := time.Unix(2000, 0)
	if err := store.AppendModerationAction(ModerationAction{
		At:                now.Add(-time.Hour),
		ChatID:            -100123,
		UserID:            55,
		MessageID:         102,
		Reason:            "zero-message",
		DeletedMessageIDs: []int{102},
		Errors:            []string{"ban user: Bad Request: not enough rights to restrict/unrestrict chat member"},
	}, cfg.ActionLogLimit); err != nil {
		t.Fatal(err)
	}

	client := &failingRetryClient{banErr: errors.New("still missing rights")}
	summary, err := RetryFailedModerationActions(cfg, store, client, now, 48*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Failed != 1 || summary.Banned != 0 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	actions := store.ModerationActions()
	retry := actions[len(actions)-1]
	if !retry.Retry || len(retry.Errors) == 0 || retry.Banned {
		t.Fatalf("unexpected retry action: %#v", retry)
	}
}

type failingRetryClient struct {
	banErr error
}

func (client *failingRetryClient) DeleteMessage(int64, int) error {
	return nil
}

func (client *failingRetryClient) BanUser(int64, int64) error {
	return client.banErr
}

func (client *failingRetryClient) SendMessage(int64, string) error {
	return nil
}

func (client *failingRetryClient) FetchAccountEvidence(int64, tgbotapi.User) AccountEvidence {
	return AccountEvidence{ChatMemberStatus: "member"}
}
