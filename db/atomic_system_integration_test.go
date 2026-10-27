//go:build integration

package db

import (
    "errors"
    "testing"

    "github.com/pagefaultgames/rogueserver/defs"
)

// Uses the disposable local MariaDB configured by setupAtomicDeleteTest.
// Run: go test -tags integration ./db -run '^TestAtomicSystem' -v -count=1
func TestAtomicSystemSaveAndDelete(t *testing.T) {
    uuid, ctx := setupAtomicDeleteTest(t)
    if _, err := handle.Exec("DELETE FROM systemSaveData WHERE uuid = ?", uuid); err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() {
        if _, err := handle.Exec("DELETE FROM systemSaveData WHERE uuid = ?", uuid); err != nil {
            t.Errorf("cleanup system data: %v", err)
        }
    })

    var trainerID, secretID int
    if err := handle.QueryRow("SELECT trainerId, secretId FROM accounts WHERE uuid = ?", uuid).Scan(&trainerID, &secretID); err != nil {
        t.Fatal(err)
    }
    // Use the account's existing IDs so the test never changes its identity.
    original := defs.SystemSaveData{
        TrainerId: trainerID, SecretId: secretID,
        Gender: 1, GameVersion: "1.12.0.10",
    }
    if err := Store.UpdateSystemAtomic(ctx, uuid, original, "delete-test-client", nil); err != nil {
        t.Fatalf("active client save failed: %v", err)
    }
    check := func(wantGender int) {
        t.Helper()
        saved, err := Store.ReadSystemSaveData(uuid)
        if err != nil { t.Fatalf("read system data: %v", err) }
        if saved.Gender != wantGender || saved.GameVersion != original.GameVersion {
            t.Fatalf("system data changed: gender=%d, version=%q", saved.Gender, saved.GameVersion)
        }
    }
    check(1)
    t.Log("PASS: active client saved system data")

    stale := original
    stale.Gender = 2
    if err := Store.UpdateSystemAtomic(ctx, uuid, stale, "old-client", nil); !errors.Is(err, ErrInactiveSaveClient) {
        t.Fatalf("expected inactive save rejection, got %v", err)
    }
    check(1)
    t.Log("PASS: inactive client cannot overwrite system data")

    if err := Store.DeleteSystemAtomic(ctx, uuid, "old-client"); !errors.Is(err, ErrInactiveSaveClient) {
        t.Fatalf("expected inactive delete rejection, got %v", err)
    }
    check(1)
    t.Log("PASS: inactive client cannot delete system data")

    if err := Store.DeleteSystemAtomic(ctx, uuid, "delete-test-client"); err != nil {
        t.Fatalf("active client delete failed: %v", err)
    }
    var count int
    if err := handle.QueryRow("SELECT COUNT(*) FROM systemSaveData WHERE uuid = ?", uuid).Scan(&count); err != nil {
        t.Fatal(err)
    }
    if count != 0 { t.Fatalf("system data still exists after deletion: %d rows", count) }
    t.Log("PASS: active client deleted system data")
}
