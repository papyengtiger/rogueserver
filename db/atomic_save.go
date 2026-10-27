package db

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"errors"
	"fmt"

	"github.com/klauspost/compress/zstd"
	"github.com/pagefaultgames/rogueserver/defs"
)

var (
	ErrStaleSessionWave   = errors.New("session out of date: existing wave index is greater")
	ErrInactiveSaveClient = errors.New("session out of date: not active")
	ErrSaveTrainerIDs     = errors.New("session out of date: stored trainer or secret ID does not match")
)

func encodeAtomicSave(v interface{}) ([]byte, error) {
	var b bytes.Buffer
	w, err := zstd.NewWriter(&b)
	if err != nil {
		return nil, err
	}
	if err = gob.NewEncoder(w).Encode(v); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func decodeAtomicSave(data []byte, v interface{}) error {
	r, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer r.Close()
	return gob.NewDecoder(r).Decode(v)
}

// SaveSessionAtomic serializes saves for one account across all server replicas.
// validateSystem is called while holding the account row lock, before any write.
// It is nil for session-only saves. A non-nil system is saved in the same transaction.
func (s *store) SaveSessionAtomic(ctx context.Context, uuid []byte, slot int, session defs.SessionSaveData, system *defs.SystemSaveData, clientSessionID string, validateSystem func(defs.SystemSaveData, bool) error) (err error) {
	if slot < 0 || slot >= defs.SessionSlotCount {
		return fmt.Errorf("invalid save slot: %d", slot)
	}
	if clientSessionID == "" {
		return ErrInactiveSaveClient
	}
	tx, err := handle.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Lock an existing account row even when this slot has never been saved.
	var trainerID, secretID int
	if err = tx.QueryRowContext(ctx, "SELECT trainerId, secretId FROM accounts WHERE uuid = ? FOR UPDATE", uuid).Scan(&trainerID, &secretID); err != nil {
		return err
	}
	var activeID string
	err = tx.QueryRowContext(ctx, "SELECT clientSessionId FROM activeClientSessions WHERE uuid = ? FOR UPDATE", uuid).Scan(&activeID)
	if errors.Is(err, sql.ErrNoRows) {
		// Match the old IsActiveSession behavior on a first-ever save.
		if _, err = tx.ExecContext(ctx, "INSERT INTO activeClientSessions (uuid, clientSessionId) VALUES (?, ?)", uuid, clientSessionID); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if activeID != "" && activeID != clientSessionID {
		return ErrInactiveSaveClient
	}
	var raw []byte
	err = tx.QueryRowContext(ctx, "SELECT data FROM sessionSaveData WHERE uuid = ? AND slot = ? FOR UPDATE", uuid, slot).Scan(&raw)
	if err == nil {
		var existing defs.SessionSaveData
		if err = decodeAtomicSave(raw, &existing); err != nil {
			return err
		}
		if existing.Seed == session.Seed && existing.WaveIndex > session.WaveIndex {
			return ErrStaleSessionWave
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if system != nil {
		if trainerID > 0 || secretID > 0 {
			if trainerID != system.TrainerId || secretID != system.SecretId {
				return ErrSaveTrainerIDs
			}
		} else {
			if _, err = tx.ExecContext(ctx, "UPDATE accounts SET trainerId = ?, secretId = ? WHERE uuid = ?", system.TrainerId, system.SecretId, uuid); err != nil {
				return err
			}
		}
		var old defs.SystemSaveData
		exists := false
		err = tx.QueryRowContext(ctx, "SELECT data FROM systemSaveData WHERE uuid = ? FOR UPDATE", uuid).Scan(&raw)
		if err == nil {
			if err = decodeAtomicSave(raw, &old); err != nil {
				return err
			}
			exists = true
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if validateSystem != nil {
			if err = validateSystem(old, exists); err != nil {
				return err
			}
		}
	}
	sessionBytes, err := encodeAtomicSave(session)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO sessionSaveData (uuid,slot,data,timestamp) VALUES (?,?,?,UTC_TIMESTAMP()) ON DUPLICATE KEY UPDATE data=VALUES(data),timestamp=VALUES(timestamp)", uuid, slot, sessionBytes); err != nil {
		return err
	}
	if system != nil {
		systemBytes, e := encodeAtomicSave(*system)
		if e != nil {
			return e
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO systemSaveData (uuid,data,timestamp) VALUES (?,?,UTC_TIMESTAMP()) ON DUPLICATE KEY UPDATE data=VALUES(data),timestamp=VALUES(timestamp)", uuid, systemBytes); err != nil {
			return err
		}
	}
	return tx.Commit()
}
