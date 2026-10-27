//go:build integration

package db

import (
    "context"
    "database/sql"
    "errors"
    "testing"

    "github.com/pagefaultgames/rogueserver/defs"
)

// Only for disposable local MariaDB at 127.0.0.1:3307/hiddenepic_test.
// Uses the existing synthetic account and slot 2. Never run against production.
func setupClearRewardsTest(t *testing.T) ([]byte, context.Context) {
    t.Helper()
    uuid, ctx := setupAtomicDeleteTest(t)
    statements := []string{
        `CREATE TABLE IF NOT EXISTS dailyRunCompletions (uuid BINARY(16) NOT NULL, seed VARCHAR(255) NOT NULL, mode INT NOT NULL, timestamp TIMESTAMP NULL, PRIMARY KEY(uuid,seed))`,
        `CREATE TABLE IF NOT EXISTS accountDailyRuns (uuid BINARY(16) NOT NULL, date DATE NOT NULL, score INT NOT NULL, wave INT NOT NULL, timestamp TIMESTAMP NULL, PRIMARY KEY(uuid,date))`,
    }
    for _, stmt := range statements {
        if _, err := handle.ExecContext(ctx, stmt); err != nil { t.Fatal(err) }
    }
    if _, err := handle.ExecContext(ctx, "DELETE FROM dailyRunCompletions WHERE uuid=? AND seed IN (?,?)", uuid, "clear-reward-unique-test", "clear-reward-daily-test"); err != nil { t.Fatal(err) }
    return uuid, ctx
}

func TestAtomicClearCompletionRewardOnce(t *testing.T) {
    uuid, ctx := setupClearRewardsTest(t)
    save := defs.SessionSaveData{Seed: "clear-reward-unique-test", GameMode: 0, WaveIndex: 200, BattleType: 2}
    // Save the immediately preceding wave to model a final battle not yet persisted.
    prior := save
    prior.WaveIndex = 199
    if err := Store.SaveSessionAtomic(ctx, uuid, 2, prior, nil, "delete-test-client", nil); err != nil { t.Fatal(err) }
    first, err := Store.ClearSessionAtomic(ctx, uuid, 2, "delete-test-client", save, "not-daily", true)
    if err != nil || !first { t.Fatalf("first completion should be recorded: success=%v err=%v", first, err) }
    var count int
    if err := handle.QueryRowContext(ctx, "SELECT COUNT(*) FROM dailyRunCompletions WHERE uuid=? AND seed=?", uuid, save.Seed).Scan(&count); err != nil { t.Fatal(err) }
    if count != 1 { t.Fatalf("expected one completion, got %d", count) }
    if _, err := Store.ReadSessionSaveData(uuid, 2); !errors.Is(err, sql.ErrNoRows) { t.Fatalf("completed save should be removed, got %v", err) }

    // A second legitimate run with the same seed must not create a second reward.
    if err := Store.SaveSessionAtomic(ctx, uuid, 2, prior, nil, "delete-test-client", nil); err != nil { t.Fatal(err) }
    second, err := Store.ClearSessionAtomic(ctx, uuid, 2, "delete-test-client", save, "not-daily", true)
    if err != nil || second { t.Fatalf("repeat completion must not award again: success=%v err=%v", second, err) }
    if err := handle.QueryRowContext(ctx, "SELECT COUNT(*) FROM dailyRunCompletions WHERE uuid=? AND seed=?", uuid, save.Seed).Scan(&count); err != nil { t.Fatal(err) }
    if count != 1 { t.Fatalf("duplicate completion inserted: %d", count) }
    t.Log("PASS: first completion rewarded once, repeat completion not rewarded")
}

func TestAtomicClearDailyRanking(t *testing.T) {
    uuid, ctx := setupClearRewardsTest(t)
    // Avoid affecting other test runs: only the synthetic account's current-day ranking.
    t.Cleanup(func() { _, _ = handle.Exec("DELETE FROM accountDailyRuns WHERE uuid=? AND date=UTC_DATE()", uuid) })
    if _, err := handle.ExecContext(ctx, "DELETE FROM accountDailyRuns WHERE uuid=? AND date=UTC_DATE()", uuid); err != nil { t.Fatal(err) }
    save := defs.SessionSaveData{Seed: "clear-reward-daily-test", GameMode: 3, WaveIndex: 50, BattleType: 2, Score: 12000}
    prior := save
    prior.WaveIndex = 49
    if err := Store.SaveSessionAtomic(ctx, uuid, 2, prior, nil, "delete-test-client", nil); err != nil { t.Fatal(err) }
    success, err := Store.ClearSessionAtomic(ctx, uuid, 2, "delete-test-client", save, save.Seed, true)
    if err != nil || !success { t.Fatalf("daily clear failed: success=%v err=%v", success, err) }
    var score, wave int
    if err := handle.QueryRowContext(ctx, "SELECT score,wave FROM accountDailyRuns WHERE uuid=? AND date=UTC_DATE()", uuid).Scan(&score,&wave); err != nil { t.Fatal(err) }
    if score != 12000 || wave != 50 { t.Fatalf("daily ranking mismatch: score=%d wave=%d", score, wave) }
    if _, err := Store.ReadSessionSaveData(uuid, 2); !errors.Is(err, sql.ErrNoRows) { t.Fatalf("daily clear did not delete save: %v", err) }
    t.Log("PASS: daily completion recorded, ranking score/wave updated, save removed")
}
