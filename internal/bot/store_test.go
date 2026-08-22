package bot

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestRecordStartupStatsSeedsFromModerationActionsAndTracksPreviousRun(t *testing.T) {
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	base := time.Unix(1000, 0).UTC()
	if err := store.RecordMessage(-100123, 55, 10, base.Add(time.Minute), 10); err != nil {
		t.Fatal(err)
	}
	actions := []ModerationAction{
		{
			At:           base.Add(2 * time.Minute),
			Reason:       "zero-message",
			DeletedCount: 2,
			Banned:       true,
		},
		{
			At:           base.Add(3 * time.Minute),
			Reason:       "crypto-keyword",
			DeletedCount: 1,
			Banned:       true,
			Retry:        true,
		},
		{
			At:     base.Add(4 * time.Minute),
			Reason: "zero-message",
			DryRun: true,
			Errors: []string{"send ban notice: unavailable"},
		},
	}
	for _, action := range actions {
		if err := store.AppendModerationAction(action, 100); err != nil {
			t.Fatal(err)
		}
	}

	firstStartup := base.Add(5 * time.Minute)
	stats, err := store.RecordStartupStats(firstStartup)
	if err != nil {
		t.Fatal(err)
	}
	if !stats.PreviousRunSeededFromAllActionLog {
		t.Fatalf("expected first startup previous run to be seeded from action log: %#v", stats)
	}
	if stats.TrackedChats != 1 || stats.TrackedUsers != 1 || stats.ObservedMessages != 1 {
		t.Fatalf("unexpected tracked totals: %#v", stats)
	}
	assertStatsSnapshot(t, "all time", stats.AllTime, ModerationStatsSnapshot{
		ModerationActions: 3,
		Bans:              2,
		DeletedMessages:   3,
		FailedActions:     1,
		DryRunActions:     1,
		RetryActions:      1,
		Reasons: []ReasonStats{
			{Reason: "zero-message", ModerationActions: 2, Bans: 1, DeletedMessages: 2},
			{Reason: "crypto-keyword", ModerationActions: 1, Bans: 1, DeletedMessages: 1},
		},
	})
	if stats.PreviousRun.ModerationActions != stats.AllTime.ModerationActions ||
		stats.PreviousRun.Bans != stats.AllTime.Bans ||
		stats.PreviousRun.DeletedMessages != stats.AllTime.DeletedMessages {
		t.Fatalf("first previous run should be seeded from all time: previous=%#v all=%#v", stats.PreviousRun, stats.AllTime)
	}

	if err := store.AppendModerationAction(ModerationAction{
		At:           base.Add(6 * time.Minute),
		Reason:       "cyrillic-heavy",
		DeletedCount: 1,
		Banned:       true,
	}, 100); err != nil {
		t.Fatal(err)
	}

	secondStartup := base.Add(7 * time.Minute)
	stats, err = store.RecordStartupStats(secondStartup)
	if err != nil {
		t.Fatal(err)
	}
	if stats.PreviousRunSeededFromAllActionLog {
		t.Fatalf("second startup should use previous startup window: %#v", stats)
	}
	if !stats.PreviousStartupAt.Equal(firstStartup) {
		t.Fatalf("previous startup = %s, want %s", stats.PreviousStartupAt, firstStartup)
	}
	assertStatsSnapshot(t, "previous run", stats.PreviousRun, ModerationStatsSnapshot{
		From:              firstStartup,
		To:                secondStartup,
		ModerationActions: 1,
		Bans:              1,
		DeletedMessages:   1,
		Reasons: []ReasonStats{
			{Reason: "cyrillic-heavy", ModerationActions: 1, Bans: 1, DeletedMessages: 1},
		},
	})
}

func assertStatsSnapshot(t *testing.T, label string, got ModerationStatsSnapshot, want ModerationStatsSnapshot) {
	t.Helper()
	if got.From != want.From ||
		got.To != want.To ||
		got.ModerationActions != want.ModerationActions ||
		got.Bans != want.Bans ||
		got.DeletedMessages != want.DeletedMessages ||
		got.FailedActions != want.FailedActions ||
		got.DryRunActions != want.DryRunActions ||
		got.RetryActions != want.RetryActions ||
		!reflect.DeepEqual(got.Reasons, want.Reasons) {
		t.Fatalf("%s stats:\nwant %#v\ngot  %#v", label, want, got)
	}
}
