//go:build integration

package db

import (
    "context"
    "errors"
    "sync"
    "testing"

    "github.com/pagefaultgames/rogueserver/defs"
)

// Run only against the disposable local MariaDB on 127.0.0.1:3307:
// go test -tags integration ./db -run TestAtomicDelete -v -count=1
func setupAtomicDeleteTest(t *testing.T) ([]byte, context.Context) {
    t.Helper()
    if err := Init("root", "hiddenepic_local_test_2026", "tcp", "127.0.0.1:3307", "hiddenepic_test"); err != nil { t.Fatal(err) }
    uuid := []byte{0x00,0x11,0x22,0x33,0x44,0x55,0x66,0x77,0x88,0x99,0xaa,0xbb,0xcc,0xdd,0xee,0xff}
    if _, err := handle.Exec("INSERT INTO activeClientSessions (uuid,clientSessionId) VALUES (?,?) ON DUPLICATE KEY UPDATE clientSessionId=VALUES(clientSessionId)", uuid, "delete-test-client"); err != nil { t.Fatal(err) }
    if _, err := handle.Exec("DELETE FROM sessionSaveData WHERE uuid=? AND slot=2", uuid); err != nil { t.Fatal(err) }
    return uuid, context.Background()
}

func TestAtomicDeleteRejectsInactiveClient(t *testing.T) {
    uuid, ctx := setupAtomicDeleteTest(t)
    original := defs.SessionSaveData{Seed:"delete-protected-seed",WaveIndex:182,Money:18200}
    if err := Store.SaveSessionAtomic(ctx,uuid,2,original,nil,"delete-test-client",nil); err != nil { t.Fatal(err) }
    err := Store.DeleteSessionAtomic(ctx,uuid,2,"old-client")
    if !errors.Is(err,ErrInactiveSaveClient) { t.Fatalf("expected inactive-client rejection, got %v",err) }
    saved,err := Store.ReadSessionSaveData(uuid,2)
    if err != nil || saved.WaveIndex != 182 || saved.Money != 18200 { t.Fatalf("save was altered: %+v, %v",saved,err) }
    t.Log("PASS: inactive client cannot delete the 182-wave save")
}

func TestAtomicDeleteClearRejectsWrongSeed(t *testing.T) {
    uuid,ctx := setupAtomicDeleteTest(t)
    original := defs.SessionSaveData{Seed:"real-seed",WaveIndex:182,Money:18200}
    if err := Store.SaveSessionAtomic(ctx,uuid,2,original,nil,"delete-test-client",nil); err != nil { t.Fatal(err) }
    fake := defs.SessionSaveData{Seed:"wrong-seed",WaveIndex:200}
    _,err := Store.ClearSessionAtomic(ctx,uuid,2,"delete-test-client",fake,"not-daily",false)
    if !errors.Is(err,ErrClearSaveMismatch) { t.Fatalf("expected mismatch rejection, got %v",err) }
    saved,err := Store.ReadSessionSaveData(uuid,2)
    if err != nil || saved.Seed != "real-seed" || saved.WaveIndex != 182 { t.Fatalf("save was altered: %+v, %v",saved,err) }
    t.Log("PASS: wrong-seed clear cannot delete the existing save")
}

func TestAtomicDeleteClearRejectsRegressedWave(t *testing.T) {
    uuid,ctx := setupAtomicDeleteTest(t)
    original := defs.SessionSaveData{Seed:"same-seed",WaveIndex:182,Money:18200}
    if err := Store.SaveSessionAtomic(ctx,uuid,2,original,nil,"delete-test-client",nil); err != nil { t.Fatal(err) }
    fake := defs.SessionSaveData{Seed:"same-seed",WaveIndex:70}
    _,err := Store.ClearSessionAtomic(ctx,uuid,2,"delete-test-client",fake,"not-daily",false)
    if !errors.Is(err,ErrClearSaveMismatch) { t.Fatalf("expected wave-regression rejection, got %v",err) }
    saved,err := Store.ReadSessionSaveData(uuid,2)
    if err != nil || saved.WaveIndex != 182 { t.Fatalf("save was altered: %+v, %v",saved,err) }
    t.Log("PASS: lower-wave clear cannot delete newer progress")
}

func TestAtomicDeleteNormalDeleteAndClear(t *testing.T) {
    uuid,ctx := setupAtomicDeleteTest(t)
    save := defs.SessionSaveData{Seed:"normal-delete",WaveIndex:40}
    if err := Store.SaveSessionAtomic(ctx,uuid,2,save,nil,"delete-test-client",nil); err != nil { t.Fatal(err) }
    if err := Store.DeleteSessionAtomic(ctx,uuid,2,"delete-test-client"); err != nil { t.Fatal(err) }
    if _,err := Store.ReadSessionSaveData(uuid,2); err == nil { t.Fatal("normal delete did not remove save") }
    save.Seed = "normal-clear"
    if err := Store.SaveSessionAtomic(ctx,uuid,2,save,nil,"delete-test-client",nil); err != nil { t.Fatal(err) }
    success,err := Store.ClearSessionAtomic(ctx,uuid,2,"delete-test-client",save,"not-daily",false)
    if err != nil { t.Fatal(err) }
    if success { t.Fatal("incomplete game should not record completion") }
    if _,err := Store.ReadSessionSaveData(uuid,2); err == nil { t.Fatal("normal clear did not remove save") }
    t.Log("PASS: normal delete and incomplete clear remove their save")
}

func TestAtomicDeleteConcurrentSaveAndDelete(t *testing.T) {
    uuid,ctx := setupAtomicDeleteTest(t)
    for i:=0;i<10;i++ {
        if _,err := handle.Exec("DELETE FROM sessionSaveData WHERE uuid=? AND slot=2",uuid);err!=nil { t.Fatal(err) }
        start:=make(chan struct{})
        var wg sync.WaitGroup
        errs:=make(chan error,2)
        wg.Add(2)
        go func(){defer wg.Done();<-start;errs<-Store.SaveSessionAtomic(ctx,uuid,2,defs.SessionSaveData{Seed:"race-seed",WaveIndex:182},nil,"delete-test-client",nil)}()
        go func(){defer wg.Done();<-start;errs<-Store.DeleteSessionAtomic(ctx,uuid,2,"delete-test-client")}()
        close(start);wg.Wait();close(errs)
        for err:=range errs { if err!=nil { t.Fatalf("iteration %d: unexpected error: %v",i,err) } }
        // Either serial order is valid: save then delete -> absent;
        // delete then save -> present at wave 182. No partial save is acceptable.
        saved,err:=Store.ReadSessionSaveData(uuid,2)
        if err==nil && (saved.Seed!="race-seed" || saved.WaveIndex!=182) { t.Fatalf("iteration %d: unexpected saved state: %+v",i,saved) }
    }
    t.Log("PASS: concurrent save/delete completed without SQL errors or partial saves")
}
