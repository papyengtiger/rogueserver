package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pagefaultgames/rogueserver/defs"
)

// UpdateSystemAtomic checks the active client, validates the existing system
// data and writes the new system data in one transaction.
func (s *store) UpdateSystemAtomic(ctx context.Context, uuid []byte, system defs.SystemSaveData, clientSessionID string, validate func(defs.SystemSaveData, bool) error) error {
	tx, err := handle.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err = lockActiveSaveAccount(ctx, tx, uuid, clientSessionID); err != nil {
		return err
	}

	var trainerID, secretID int
	if err = tx.QueryRowContext(ctx, "SELECT trainerId, secretId FROM accounts WHERE uuid = ?", uuid).Scan(&trainerID, &secretID); err != nil {
		return err
	}
	if trainerID > 0 || secretID > 0 {
		if trainerID != system.TrainerId || secretID != system.SecretId {
			return ErrSaveTrainerIDs
		}
	} else {
		if _, err = tx.ExecContext(ctx, "UPDATE accounts SET trainerId = ?, secretId = ? WHERE uuid = ?", system.TrainerId, system.SecretId, uuid); err != nil {
			return err
		}
	}

	var raw []byte
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

	if validate != nil {
		if err = validate(old, exists); err != nil {
			return err
		}
	}

	encoded, err := encodeAtomicSave(system)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO systemSaveData (uuid,data,timestamp) VALUES (?,?,UTC_TIMESTAMP()) ON DUPLICATE KEY UPDATE data=VALUES(data),timestamp=VALUES(timestamp)", uuid, encoded); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteSystemAtomic rejects deletion from an inactive client.
func (s *store) DeleteSystemAtomic(ctx context.Context, uuid []byte, clientSessionID string) error {
	tx, err := handle.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err = lockActiveSaveAccount(ctx, tx, uuid, clientSessionID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM systemSaveData WHERE uuid = ?", uuid); err != nil {
		return err
	}
	return tx.Commit()
}
