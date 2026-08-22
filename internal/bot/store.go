package bot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

type FileStore struct {
	mu    sync.Mutex
	path  string
	state persistentState
}

type persistentState struct {
	Chats             map[string]map[string]*UserHistory `json:"chats"`
	ModerationActions []ModerationAction                 `json:"moderation_actions,omitempty"`
	Stats             *StartupStatsState                 `json:"stats,omitempty"`
}

type StartupStatsState struct {
	LastStartupAt time.Time               `json:"last_startup_at,omitempty"`
	AllTime       ModerationStatsSnapshot `json:"all_time"`
	PreviousRun   ModerationStatsSnapshot `json:"previous_run"`
}

type StartupStats struct {
	StartupAt                         time.Time
	PreviousStartupAt                 time.Time
	PreviousRunSeededFromAllActionLog bool
	TrackedChats                      int
	TrackedUsers                      int
	ObservedMessages                  int
	AllTime                           ModerationStatsSnapshot
	PreviousRun                       ModerationStatsSnapshot
}

type ModerationStatsSnapshot struct {
	From              time.Time     `json:"from,omitempty"`
	To                time.Time     `json:"to,omitempty"`
	ModerationActions int           `json:"moderation_actions"`
	Bans              int           `json:"bans"`
	DeletedMessages   int           `json:"deleted_messages"`
	FailedActions     int           `json:"failed_actions"`
	DryRunActions     int           `json:"dry_run_actions"`
	RetryActions      int           `json:"retry_actions"`
	Reasons           []ReasonStats `json:"reasons,omitempty"`
}

type ReasonStats struct {
	Reason            string `json:"reason"`
	ModerationActions int    `json:"moderation_actions"`
	Bans              int    `json:"bans"`
	DeletedMessages   int    `json:"deleted_messages"`
}

type UserHistory struct {
	MessageCount     int       `json:"message_count"`
	FirstSeenAt      time.Time `json:"first_seen_at,omitempty"`
	JoinedAt         time.Time `json:"joined_at,omitempty"`
	LastMessageAt    time.Time `json:"last_message_at,omitempty"`
	LastModeratedAt  time.Time `json:"last_moderated_at,omitempty"`
	LastModeration   string    `json:"last_moderation,omitempty"`
	RecentMessageIDs []int     `json:"recent_message_ids,omitempty"`
}

type ModerationAction struct {
	At                         time.Time       `json:"at"`
	ChatID                     int64           `json:"chat_id"`
	ChatType                   string          `json:"chat_type,omitempty"`
	ChatTitle                  string          `json:"chat_title,omitempty"`
	ChatUsername               string          `json:"chat_username,omitempty"`
	UserID                     int64           `json:"user_id"`
	User                       AccountEvidence `json:"user"`
	MessageID                  int             `json:"message_id"`
	MessageDate                time.Time       `json:"message_date"`
	MessageTextSample          string          `json:"message_text_sample,omitempty"`
	ObservedMessageCountBefore int             `json:"observed_message_count_before"`
	ObservedFirstSeenAt        time.Time       `json:"observed_first_seen_at,omitempty"`
	ObservedJoinedAt           time.Time       `json:"observed_joined_at,omitempty"`
	RecentMessageIDs           []int           `json:"recent_message_ids,omitempty"`
	Reason                     string          `json:"reason"`
	DeletedMessageIDs          []int           `json:"deleted_message_ids,omitempty"`
	DeletedCount               int             `json:"deleted_count"`
	Banned                     bool            `json:"banned"`
	DryRun                     bool            `json:"dry_run"`
	Retry                      bool            `json:"retry,omitempty"`
	RetryOf                    time.Time       `json:"retry_of,omitempty"`
	TelegramLimitations        []string        `json:"telegram_limitations,omitempty"`
	Errors                     []string        `json:"errors,omitempty"`
}

type AccountEvidence struct {
	ID                      int64     `json:"id"`
	FirstName               string    `json:"first_name,omitempty"`
	LastName                string    `json:"last_name,omitempty"`
	Username                string    `json:"username,omitempty"`
	LanguageCode            string    `json:"language_code,omitempty"`
	IsBot                   bool      `json:"is_bot,omitempty"`
	BotAPIAccountCreatedAt  string    `json:"bot_api_account_created_at"`
	BotAPIDescription       string    `json:"bot_api_description"`
	ObservedFirstSeenAt     time.Time `json:"observed_first_seen_at,omitempty"`
	ObservedJoinedAt        time.Time `json:"observed_joined_at,omitempty"`
	ChatMemberStatus        string    `json:"chat_member_status,omitempty"`
	ChatMemberCustomTitle   string    `json:"chat_member_custom_title,omitempty"`
	ChatMemberUntilDate     int64     `json:"chat_member_until_date,omitempty"`
	ChatMemberCanDelete     bool      `json:"chat_member_can_delete_messages,omitempty"`
	ChatMemberCanRestrict   bool      `json:"chat_member_can_restrict_members,omitempty"`
	ChatMemberCanManageChat bool      `json:"chat_member_can_manage_chat,omitempty"`
	ChatMemberCanPromote    bool      `json:"chat_member_can_promote_members,omitempty"`
	ChatMemberLookupError   string    `json:"chat_member_lookup_error,omitempty"`
	ChatMemberRawJSON       string    `json:"chat_member_raw_json,omitempty"`
}

func OpenFileStore(path string) (*FileStore, error) {
	store := &FileStore{
		path: path,
		state: persistentState{
			Chats: make(map[string]map[string]*UserHistory),
		},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return store, nil
	}
	if err := json.Unmarshal(data, &store.state); err != nil {
		return nil, err
	}
	if store.state.Chats == nil {
		store.state.Chats = make(map[string]map[string]*UserHistory)
	}
	return store, nil
}

func (store *FileStore) MarkJoin(chatID int64, userID int64, at time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	history := store.historyLocked(chatID, userID)
	if history.FirstSeenAt.IsZero() {
		history.FirstSeenAt = at
	}
	history.JoinedAt = at
	return store.saveLocked()
}

func (store *FileStore) History(chatID int64, userID int64) HistoryView {
	store.mu.Lock()
	defer store.mu.Unlock()

	history := store.historyLocked(chatID, userID)
	return HistoryView{
		MessageCount:     history.MessageCount,
		FirstSeenAt:      history.FirstSeenAt,
		JoinedAt:         history.JoinedAt,
		RecentMessageIDs: append([]int(nil), history.RecentMessageIDs...),
	}
}

func (store *FileStore) RecordMessage(chatID int64, userID int64, messageID int, at time.Time, recentLimit int) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	history := store.historyLocked(chatID, userID)
	if history.FirstSeenAt.IsZero() {
		history.FirstSeenAt = at
	}
	history.MessageCount++
	history.LastMessageAt = at
	history.RecentMessageIDs = appendRecentMessage(history.RecentMessageIDs, messageID, recentLimit)
	return store.saveLocked()
}

func (store *FileStore) MarkModerated(chatID int64, userID int64, messageID int, at time.Time, reason string, recentLimit int) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	history := store.historyLocked(chatID, userID)
	if history.FirstSeenAt.IsZero() {
		history.FirstSeenAt = at
	}
	history.MessageCount++
	history.LastMessageAt = at
	history.LastModeratedAt = at
	history.LastModeration = reason
	history.RecentMessageIDs = appendRecentMessage(history.RecentMessageIDs, messageID, recentLimit)
	return store.saveLocked()
}

func (store *FileStore) AppendModerationAction(action ModerationAction, limit int) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.state.ModerationActions = append(store.state.ModerationActions, action)
	if limit > 0 && len(store.state.ModerationActions) > limit {
		store.state.ModerationActions = store.state.ModerationActions[len(store.state.ModerationActions)-limit:]
	}
	return store.saveLocked()
}

func (store *FileStore) ModerationActions() []ModerationAction {
	store.mu.Lock()
	defer store.mu.Unlock()

	return append([]ModerationAction(nil), store.state.ModerationActions...)
}

func (store *FileStore) RecordStartupStats(now time.Time) (StartupStats, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	if now.IsZero() {
		now = time.Now()
	}
	if store.state.Stats == nil {
		store.state.Stats = &StartupStatsState{}
	}

	previousStartupAt := store.state.Stats.LastStartupAt
	allTime := moderationStatsSnapshot(store.state.ModerationActions, time.Time{}, time.Time{})
	previousRun := moderationStatsSnapshot(store.state.ModerationActions, previousStartupAt, now)
	seededFromAllActionLog := previousStartupAt.IsZero()
	if seededFromAllActionLog {
		previousRun = allTime
		previousRun.To = now
	}

	trackedChats, trackedUsers, observedMessages := store.trackedTotalsLocked()
	stats := StartupStats{
		StartupAt:                         now,
		PreviousStartupAt:                 previousStartupAt,
		PreviousRunSeededFromAllActionLog: seededFromAllActionLog,
		TrackedChats:                      trackedChats,
		TrackedUsers:                      trackedUsers,
		ObservedMessages:                  observedMessages,
		AllTime:                           allTime,
		PreviousRun:                       previousRun,
	}

	store.state.Stats.LastStartupAt = now
	store.state.Stats.AllTime = allTime
	store.state.Stats.PreviousRun = previousRun
	if err := store.saveLocked(); err != nil {
		return StartupStats{}, err
	}
	return stats, nil
}

func moderationStatsSnapshot(actions []ModerationAction, from time.Time, to time.Time) ModerationStatsSnapshot {
	stats := ModerationStatsSnapshot{
		From: from,
		To:   to,
	}
	byReason := make(map[string]*ReasonStats)
	for _, action := range actions {
		if !from.IsZero() && action.At.Before(from) {
			continue
		}
		if !to.IsZero() && !action.At.Before(to) {
			continue
		}
		stats.ModerationActions++
		if action.Banned {
			stats.Bans++
		}
		stats.DeletedMessages += action.DeletedCount
		if len(action.Errors) > 0 {
			stats.FailedActions++
		}
		if action.DryRun {
			stats.DryRunActions++
		}
		if action.Retry {
			stats.RetryActions++
		}
		reason := action.Reason
		if reason == "" {
			reason = "unknown"
		}
		reasonStats := byReason[reason]
		if reasonStats == nil {
			reasonStats = &ReasonStats{Reason: reason}
			byReason[reason] = reasonStats
		}
		reasonStats.ModerationActions++
		if action.Banned {
			reasonStats.Bans++
		}
		reasonStats.DeletedMessages += action.DeletedCount
	}
	if len(byReason) > 0 {
		reasons := make([]ReasonStats, 0, len(byReason))
		for _, reasonStats := range byReason {
			reasons = append(reasons, *reasonStats)
		}
		sort.Slice(reasons, func(i, j int) bool {
			if reasons[i].ModerationActions != reasons[j].ModerationActions {
				return reasons[i].ModerationActions > reasons[j].ModerationActions
			}
			return reasons[i].Reason < reasons[j].Reason
		})
		stats.Reasons = reasons
	}
	return stats
}

func (store *FileStore) trackedTotalsLocked() (int, int, int) {
	trackedChats := 0
	trackedUsers := 0
	observedMessages := 0
	for _, users := range store.state.Chats {
		if len(users) == 0 {
			continue
		}
		trackedChats++
		trackedUsers += len(users)
		for _, history := range users {
			if history != nil {
				observedMessages += history.MessageCount
			}
		}
	}
	return trackedChats, trackedUsers, observedMessages
}

func (store *FileStore) historyLocked(chatID int64, userID int64) *UserHistory {
	chatKey := strconv.FormatInt(chatID, 10)
	userKey := strconv.FormatInt(userID, 10)

	users := store.state.Chats[chatKey]
	if users == nil {
		users = make(map[string]*UserHistory)
		store.state.Chats[chatKey] = users
	}
	history := users[userKey]
	if history == nil {
		history = &UserHistory{}
		users[userKey] = history
	}
	return history
}

func appendRecentMessage(ids []int, messageID int, limit int) []int {
	if messageID <= 0 {
		return ids
	}
	if limit < 1 {
		limit = defaultDeleteRecentLimit
	}
	for _, id := range ids {
		if id == messageID {
			return ids
		}
	}
	ids = append(ids, messageID)
	if len(ids) > limit {
		ids = ids[len(ids)-limit:]
	}
	return ids
}

func (store *FileStore) saveLocked() error {
	if store.path == "" {
		return fmt.Errorf("state path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(store.path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := store.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, store.path)
}
