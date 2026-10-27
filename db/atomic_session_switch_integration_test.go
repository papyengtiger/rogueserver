//go:build integration

package db

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "testing"
    "time"

    "github.com/pagefaultgames/rogueserver/defs"
)

// These tests use only the disposable local MariaDB configured by
// setupAtomicDeleteTest in atomic_delete_integration_test.go.
func TestAtomicSessionSwitchWaitsForAccountLock(t *testing.T) {
    uuid, _ := setupAtomicDeleteTest(t)
    if err := Store.UpdateActiveSession(uuid, "switch-A"); err != nil { t.Fatal(err) }

    tx, err := handle.BeginTx(context.Background(), nil)
    if err != nil { t.Fatal(err) }
    locked := true
    defer func() { if locked { _ = tx.Rollback() } }()
    var id []byte
    if err := tx.QueryRow("SELECT uuid FROM accounts WHERE uuid = ? FOR UPDATE", uuid).Scan(&id); err != nil { t.Fatal(err) }

    done := make(chan error, 1)
    go func() { done <- Store.UpdateActiveSession(uuid, "switch-B") }()
    select {
    case err := <-done:
        t.Fatalf("session switch completed while account row was locked: %v", err)
    case <-time.After(150 * time.Millisecond):
        // The switch is blocked on the same account row lock.
    }
    if err := tx.Commit(); err != nil { t.Fatal(err) }
    locked = false
    select {
    case err := <-done:
        if err != nil { t.Fatal(err) }
    case <-time.After(5 * time.Second):
        t.Fatal("session switch did not complete after releasing the account lock")
    }
    active, err := Store.IsActiveSession(uuid, "switch-B")
    if err != nil || !active { t.Fatalf("switch-B not active: active=%v err=%v", active, err) }
    t.Log("PASS: session switch waited for the account row lock")
}

func TestAtomicSessionSwitchConcurrentSave(t *testing.T) {
    uuid, ctx := setupAtomicDeleteTest(t)
    const slot = 2
    for i := 0; i < 20; i++ {
        if err := Store.UpdateActiveSession(uuid, "switch-A"); err != nil { t.Fatal(err) }
        seed := fmt.Sprintf("session-switch-race-%d", i)
        baseline := defs.SessionSaveData{Seed: seed, WaveIndex: 70, Money: 7000}
        if err := Store.SaveSessionAtomic(ctx, uuid, slot, baseline, nil, "switch-A", nil); err != nil { t.Fatal(err) }

        start := make(chan struct{})
        var wg sync.WaitGroup
        var saveErr, switchErr error
        wg.Add(2)
        go func() {
            defer wg.Done()
            <-start
            newer := defs.SessionSaveData{Seed: seed, WaveIndex: 182, Money: 18200}
            saveErr = Store.SaveSessionAtomic(ctx, uuid, slot, newer, nil, "switch-A", nil)
        }()
        go func() {
            defer wg.Done()
            <-start
            switchErr = Store.UpdateActiveSession(uuid, "switch-B")
        }()
        close(start)
        wg.Wait()
        if switchErr != nil { t.Fatalf("round %d: switch error: %v", i, switchErr) }
        if saveErr != nil && !errors.Is(saveErr, ErrInactiveSaveClient) {
            t.Fatalf("round %d: unexpected save error: %v", i, saveErr)
        }
        active, err := Store.IsActiveSession(uuid, "switch-B")
        if err != nil || !active { t.Fatalf("round %d: B not active: %v", i, err) }
        stale := defs.SessionSaveData{Seed: seed, WaveIndex: 183, Money: 18300}
        if err := Store.SaveSessionAtomic(ctx, uuid, slot, stale, nil, "switch-A", nil); !errors.Is(err, ErrInactiveSaveClient) {
            t.Fatalf("round %d: old client was not rejected after switch: %v", i, err)
        }
        saved, err := Store.ReadSessionSaveData(uuid, slot)
        if err != nil { t.Fatalf("round %d: read error: %v", i, err) }
        if saved.Seed != seed || !((saved.WaveIndex == 70 && saved.Money == 7000) || (saved.WaveIndex == 182 && saved.Money == 18200)) {
            t.Fatalf("round %d: unexpected save: seed=%q wave=%d money=%d", i, saved.Seed, saved.WaveIndex, saved.Money)
        }
    }
    t.Log("PASS: 20 save/switch races; B active, stale A rejected, saved progress consistent")
}
