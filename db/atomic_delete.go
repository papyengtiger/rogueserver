package db

import (
    "context"
    "database/sql"
    "errors"
    "fmt"

    "github.com/pagefaultgames/rogueserver/defs"
)

var ErrClearSaveMismatch = errors.New("session out of date: clear does not match saved session")
var ErrSaveSlotMissing = errors.New("session out of date: save slot does not exist")

// lockActiveSaveAccount must be called inside a transaction. It shares the
// accounts row lock used by SaveSessionAtomic across all server replicas.
func lockActiveSaveAccount(ctx context.Context, tx *sql.Tx, uuid []byte, clientSessionID string) error {
    if clientSessionID == "" { return ErrInactiveSaveClient }
    var id []byte
    if err := tx.QueryRowContext(ctx, "SELECT uuid FROM accounts WHERE uuid = ? FOR UPDATE", uuid).Scan(&id); err != nil { return err }
    var active string
    err := tx.QueryRowContext(ctx, "SELECT clientSessionId FROM activeClientSessions WHERE uuid = ? FOR UPDATE", uuid).Scan(&active)
    if errors.Is(err, sql.ErrNoRows) { return ErrInactiveSaveClient }
    if err != nil { return err }
    if active != clientSessionID { return ErrInactiveSaveClient }
    return nil
}

func (s *store) DeleteSessionAtomic(ctx context.Context, uuid []byte, slot int, clientSessionID string) error {
    if slot < 0 || slot >= defs.SessionSlotCount { return fmt.Errorf("invalid save slot: %d", slot) }
    tx, err := handle.BeginTx(ctx, nil)
    if err != nil { return err }
    defer tx.Rollback()
    if err = lockActiveSaveAccount(ctx, tx, uuid, clientSessionID); err != nil { return err }
    if _, err = tx.ExecContext(ctx, "DELETE FROM sessionSaveData WHERE uuid = ? AND slot = ?", uuid, slot); err != nil { return err }
    return tx.Commit()
}

// ClearSessionAtomic verifies the stored game and commits completion, daily
// ranking, and deletion together. The caller computes completed using the
// existing game-mode rules; the database rechecks the save identity.
func (s *store) ClearSessionAtomic(ctx context.Context, uuid []byte, slot int, clientSessionID string, save defs.SessionSaveData, dailySeed string, completed bool) (bool, error) {
    if slot < 0 || slot >= defs.SessionSlotCount { return false, fmt.Errorf("invalid save slot: %d", slot) }
    tx, err := handle.BeginTx(ctx, nil)
    if err != nil { return false, err }
    defer tx.Rollback()
    if err = lockActiveSaveAccount(ctx, tx, uuid, clientSessionID); err != nil { return false, err }
    var raw []byte
    err = tx.QueryRowContext(ctx, "SELECT data FROM sessionSaveData WHERE uuid = ? AND slot = ? FOR UPDATE", uuid, slot).Scan(&raw)
    if errors.Is(err, sql.ErrNoRows) { return false, ErrSaveSlotMissing }
    if err != nil { return false, err }
    var stored defs.SessionSaveData
    if err = decodeAtomicSave(raw, &stored); err != nil { return false, err }
    // The final battle may not have been saved yet; allow forward progress,
    // but never accept a different game or a regressed wave.
    if stored.Seed != save.Seed || stored.GameMode != save.GameMode || stored.WaveIndex > save.WaveIndex {
        return false, ErrClearSaveMismatch
    }
    if save.GameMode == 3 && save.Seed == dailySeed {
        wave := save.WaveIndex
        if !completed { wave-- }
        if save.Score >= 20000 {
            if _, err = tx.ExecContext(ctx, "UPDATE accounts SET banned = 1 WHERE uuid = ?", uuid); err != nil { return false, err }
        }
        if _, err = tx.ExecContext(ctx, "INSERT INTO accountDailyRuns (uuid, date, score, wave, timestamp) VALUES (?, UTC_DATE(), ?, ?, UTC_TIMESTAMP()) ON DUPLICATE KEY UPDATE score = GREATEST(score, ?), wave = GREATEST(wave, ?), timestamp = IF(score < ?, UTC_TIMESTAMP(), timestamp)", uuid, save.Score, wave, save.Score, wave, save.Score); err != nil { return false, err }
    }
    success := false
    if completed {
        var count int
        if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM dailyRunCompletions WHERE uuid = ? AND seed = ?", uuid, save.Seed).Scan(&count); err != nil { return false, err }
        if count == 0 {
            if _, err = tx.ExecContext(ctx, "INSERT INTO dailyRunCompletions (uuid, seed, mode, timestamp) VALUES (?, ?, ?, UTC_TIMESTAMP())", uuid, save.Seed, int(save.GameMode)); err != nil { return false, err }
            success = true
        }
    }
    if _, err = tx.ExecContext(ctx, "DELETE FROM sessionSaveData WHERE uuid = ? AND slot = ?", uuid, slot); err != nil { return false, err }
    if err = tx.Commit(); err != nil { return false, err }
    return success, nil
}
