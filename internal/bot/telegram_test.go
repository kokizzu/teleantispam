package bot

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type fakeTelegramClient struct {
	memberStatus      string
	memberLookupError string
	deleted           []int
	banned            []int64
	sent              []sentMessage
}

type sentMessage struct {
	chatID int64
	text   string
}

func (client *fakeTelegramClient) DeleteMessage(_ int64, messageID int) error {
	client.deleted = append(client.deleted, messageID)
	return nil
}

func (client *fakeTelegramClient) BanUser(_ int64, userID int64) error {
	client.banned = append(client.banned, userID)
	return nil
}

func (client *fakeTelegramClient) SendMessage(chatID int64, text string) error {
	client.sent = append(client.sent, sentMessage{chatID: chatID, text: text})
	return nil
}

func (client *fakeTelegramClient) FetchAccountEvidence(_ int64, user tgbotapi.User) AccountEvidence {
	evidence := AccountEvidenceFromUser(user)
	evidence.ChatMemberLookupError = client.memberLookupError
	if client.memberStatus == "" {
		evidence.ChatMemberStatus = "member"
	} else {
		evidence.ChatMemberStatus = client.memberStatus
	}
	return evidence
}

func TestHandleMessageDeletesRecentMessagesBansLogsAndNotifies(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	client := &fakeTelegramClient{}
	now := time.Unix(1000, 0)
	if err := store.MarkJoin(-100123, 55, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	for _, id := range []int{100, 101} {
		err := HandleMessage(cfg, store, client, &tgbotapi.Message{
			MessageID: id,
			Date:      int(now.Unix()),
			Chat:      &tgbotapi.Chat{ID: -100123, Type: "supergroup", Title: "Gophers ID"},
			From:      &tgbotapi.User{ID: 55, FirstName: "Jeana", UserName: "spammy"},
			Text:      "hello",
		}, now)
		if err != nil {
			t.Fatalf("record seed message %d: %v", id, err)
		}
	}

	err = HandleMessage(cfg, store, client, &tgbotapi.Message{
		MessageID: 102,
		Date:      int(now.Unix()),
		Chat:      &tgbotapi.Chat{ID: -100123, Type: "supergroup", Title: "Gophers ID"},
		From:      &tgbotapi.User{ID: 55, FirstName: "Jeana", UserName: "spammy"},
		Text:      "0",
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(client.deleted, []int{100, 101, 102}) {
		t.Fatalf("unexpected deleted messages: %#v", client.deleted)
	}
	if !reflect.DeepEqual(client.banned, []int64{55}) {
		t.Fatalf("unexpected banned users: %#v", client.banned)
	}
	if len(client.sent) != 1 {
		t.Fatalf("expected one notice, got %#v", client.sent)
	}
	if client.sent[0].chatID != -100123 {
		t.Fatalf("expected source chat notice, got %#v", client.sent[0])
	}
	expectedNotice := "55 / Jeana / @spammy is banned because zero-message, 3 messages deleted"
	if client.sent[0].text != expectedNotice {
		t.Fatalf("unexpected notice:\nwant %q\ngot  %q", expectedNotice, client.sent[0].text)
	}

	actions := store.ModerationActions()
	if len(actions) != 1 {
		t.Fatalf("expected one action log entry, got %#v", actions)
	}
	action := actions[0]
	if action.User.ID != 55 || action.User.Username != "spammy" || action.User.ChatMemberStatus != "member" {
		t.Fatalf("expected account evidence in action log, got %#v", action.User)
	}
	if !strings.Contains(action.User.BotAPIAccountCreatedAt, "does not expose") {
		t.Fatalf("expected creation-time limitation evidence, got %#v", action.User.BotAPIAccountCreatedAt)
	}
	if action.DeletedCount != 3 || !action.Banned || action.Reason != "zero-message" {
		t.Fatalf("unexpected action log entry: %#v", action)
	}
}

func TestHandleMessageDoesNotModerateObservedTenPostUser(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	client := &fakeTelegramClient{}
	now := time.Unix(1000, 0)

	for id := 1; id <= 10; id++ {
		err := HandleMessage(cfg, store, client, &tgbotapi.Message{
			MessageID: id,
			Date:      int(now.Unix()),
			Chat:      &tgbotapi.Chat{ID: -100123},
			From:      &tgbotapi.User{ID: 77, FirstName: "Old"},
			Text:      "normal message",
		}, now)
		if err != nil {
			t.Fatalf("record seed message %d: %v", id, err)
		}
	}

	err = HandleMessage(cfg, store, client, &tgbotapi.Message{
		MessageID: 11,
		Date:      int(now.Unix()),
		Chat:      &tgbotapi.Chat{ID: -100123},
		From:      &tgbotapi.User{ID: 77, FirstName: "Old"},
		Text:      "0 bitcoin крипто 免费",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.deleted) != 0 || len(client.banned) != 0 || len(store.ModerationActions()) != 0 {
		t.Fatalf(">=10 observed posts must not be moderated; deleted=%#v banned=%#v actions=%#v", client.deleted, client.banned, store.ModerationActions())
	}
}

func TestHandleMessageDoesNotModerateAdministrator(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	client := &fakeTelegramClient{memberStatus: "administrator"}
	now := time.Unix(1000, 0)

	err = HandleMessage(cfg, store, client, &tgbotapi.Message{
		MessageID: 1,
		Date:      int(now.Unix()),
		Chat:      &tgbotapi.Chat{ID: -100123},
		From:      &tgbotapi.User{ID: 77, FirstName: "Admin"},
		Text:      "0 bitcoin крипто 免费",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.deleted) != 0 || len(client.banned) != 0 || len(store.ModerationActions()) != 0 {
		t.Fatalf("administrator must not be moderated; deleted=%#v banned=%#v actions=%#v", client.deleted, client.banned, store.ModerationActions())
	}
}

func TestHandleMessageDoesNotModerateCreator(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	client := &fakeTelegramClient{memberStatus: "creator"}
	now := time.Unix(1000, 0)

	err = HandleMessage(cfg, store, client, &tgbotapi.Message{
		MessageID: 1,
		Date:      int(now.Unix()),
		Chat:      &tgbotapi.Chat{ID: -100123},
		From:      &tgbotapi.User{ID: 78, FirstName: "Creator"},
		Text:      "0 bitcoin крипто 免费",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.deleted) != 0 || len(client.banned) != 0 || len(store.ModerationActions()) != 0 {
		t.Fatalf("creator must not be moderated; deleted=%#v banned=%#v actions=%#v", client.deleted, client.banned, store.ModerationActions())
	}
}

func TestHandleMessageDoesNotModerateWhenMemberLookupFails(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	client := &fakeTelegramClient{memberLookupError: "telegram unavailable"}
	now := time.Unix(1000, 0)

	err = HandleMessage(cfg, store, client, &tgbotapi.Message{
		MessageID: 1,
		Date:      int(now.Unix()),
		Chat:      &tgbotapi.Chat{ID: -100123},
		From:      &tgbotapi.User{ID: 79, FirstName: "Unknown"},
		Text:      "0 bitcoin крипто 免费",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.deleted) != 0 || len(client.banned) != 0 || len(store.ModerationActions()) != 0 {
		t.Fatalf("unverified member must not be moderated; deleted=%#v banned=%#v actions=%#v", client.deleted, client.banned, store.ModerationActions())
	}
}

func TestProtectedChatMemberStatus(t *testing.T) {
	protected := []string{"creator", "administrator"}
	for _, status := range protected {
		if !protectedChatMemberStatus(status) {
			t.Fatalf("expected %q to be protected", status)
		}
	}
	unprotected := []string{"member", "restricted", "left", "kicked", ""}
	for _, status := range unprotected {
		if protectedChatMemberStatus(status) {
			t.Fatalf("expected %q to be unprotected", status)
		}
	}
}

func TestHandleUpdatesProcessesRecentPendingAndSkipsOld(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	client := &fakeTelegramClient{}
	now := time.Unix(200000, 0)

	nextOffset, err := HandleUpdates(cfg, store, client, []tgbotapi.Update{
		{
			UpdateID: 20,
			Message: &tgbotapi.Message{
				MessageID: 1,
				Date:      int(now.Add(-25 * time.Hour).Unix()),
				Chat:      &tgbotapi.Chat{ID: -100123},
				From:      &tgbotapi.User{ID: 88, FirstName: "OldPending"},
				Text:      "0",
			},
		},
		{
			UpdateID: 21,
			Message: &tgbotapi.Message{
				MessageID: 2,
				Date:      int(now.Add(-time.Hour).Unix()),
				Chat:      &tgbotapi.Chat{ID: -100123},
				NewChatMembers: []tgbotapi.User{
					{ID: 89, FirstName: "NewPending"},
				},
			},
		},
		{
			UpdateID: 22,
			Message: &tgbotapi.Message{
				MessageID: 3,
				Date:      int(now.Add(-time.Hour).Unix()),
				Chat:      &tgbotapi.Chat{ID: -100123},
				From:      &tgbotapi.User{ID: 89, FirstName: "NewPending"},
				Text:      "0",
			},
		},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if nextOffset != 23 {
		t.Fatalf("unexpected next offset: %d", nextOffset)
	}
	if !reflect.DeepEqual(client.banned, []int64{89}) {
		t.Fatalf("expected only recent pending spammer banned, got %#v", client.banned)
	}
}

func TestHandleMessageDryRunLogsButDoesNotDeleteOrBanOrNotify(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.DryRun = true
	client := &fakeTelegramClient{}
	if err := store.MarkJoin(-100123, 99, time.Unix(990, 0)); err != nil {
		t.Fatal(err)
	}

	err = HandleMessage(cfg, store, client, &tgbotapi.Message{
		MessageID: 1,
		Date:      1000,
		Chat:      &tgbotapi.Chat{ID: -100123},
		From:      &tgbotapi.User{ID: 99, FirstName: "Dry"},
		Text:      "0",
	}, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(client.deleted) != 0 || len(client.banned) != 0 || len(client.sent) != 0 {
		t.Fatalf("dry run should not moderate externally, deleted=%#v banned=%#v sent=%#v", client.deleted, client.banned, client.sent)
	}
	if len(store.ModerationActions()) != 1 || !store.ModerationActions()[0].DryRun {
		t.Fatalf("dry run should still log evidence, got %#v", store.ModerationActions())
	}
}

func TestBanUserConfigRevokesMessages(t *testing.T) {
	cfg := banUserConfig(-100123, 55)
	if cfg.ChatID != -100123 || cfg.UserID != 55 {
		t.Fatalf("unexpected ban target: %#v", cfg)
	}
	if !cfg.RevokeMessages {
		t.Fatalf("expected ban config to revoke messages: %#v", cfg)
	}
}
