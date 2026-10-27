package db

import (
    "context"
    "errors"
    "fmt"
    "testing"

    "github.com/pagefaultgames/rogueserver/defs"
)

func TestAtomicSaveRejectsOlderWave(t *testing.T) {
    if err := Init("root", "hiddenepic_local_test_2026", "tcp", "127.0.0.1:3307", "hiddenepic_test"); err != nil {
        t.Fatal(err)
    }

    uuid := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
    if err := Store.UpdateActiveSession(uuid, "test-client"); err != nil { t.Fatal(err) }
    ctx := context.Background()

    newer := defs.SessionSaveData{Seed: "hiddenepic-test-seed", WaveIndex: 182, Money: 18200}
    older := defs.SessionSaveData{Seed: "hiddenepic-test-seed", WaveIndex: 70, Money: 7000}

    if err := Store.SaveSessionAtomic(ctx, uuid, 0, newer, nil, "test-client", nil); err != nil {
        t.Fatalf("182층 저장 실패: %v", err)
    }

    err := Store.SaveSessionAtomic(ctx, uuid, 0, older, nil, "test-client", nil)
    if !errors.Is(err, ErrStaleSessionWave) {
        t.Fatalf("70층 저장이 거부되어야 합니다. 실제 오류: %v", err)
    }

    saved, err := Store.ReadSessionSaveData(uuid, 0)
    if err != nil {
        t.Fatal(err)
    }
    if saved.WaveIndex != 182 || saved.Money != 18200 {
        t.Fatalf("저장 데이터가 변경됐습니다: 층수=%d, 돈=%d", saved.WaveIndex, saved.Money)
    }

    t.Logf("성공: 70층 저장 거부, 기존 %d층 및 돈 %d 유지", saved.WaveIndex, saved.Money)
}

// TestAtomicSaveConcurrentWaves checks that a lower wave cannot overwrite a
// higher wave even when two saves race to acquire the account row lock.
func TestAtomicSaveConcurrentWaves(t *testing.T) {
    if err := Init("root", "hiddenepic_local_test_2026", "tcp", "127.0.0.1:3307", "hiddenepic_test"); err != nil {
        t.Fatal(err)
    }
    uuid := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
    if err := Store.UpdateActiveSession(uuid, "test-client"); err != nil { t.Fatal(err) }
    ctx := context.Background()
    for i := 0; i < 30; i++ {
        seed := fmt.Sprintf("concurrent-save-test-%d", i)
        start := make(chan struct{})
        type result struct {
            wave int
            err error
        }
        results := make(chan result, 2)
        for _, wave := range []int{70, 182} {
            go func(wave int) {
                <-start
                data := defs.SessionSaveData{Seed: seed, WaveIndex: wave, Money: wave * 100}
                err := Store.SaveSessionAtomic(ctx, uuid, 1, data, nil, "test-client", nil)
                results <- result{wave: wave, err: err}
            }(wave)
        }
        close(start)
        for j := 0; j < 2; j++ {
            r := <-results
            if r.wave == 182 && r.err != nil {
                t.Fatalf("반복 %d: 182층 저장 실패: %v", i, r.err)
            }
            if r.wave == 70 && r.err != nil && !errors.Is(r.err, ErrStaleSessionWave) {
                t.Fatalf("반복 %d: 70층 저장에서 예상하지 못한 오류: %v", i, r.err)
            }
        }
        saved, err := Store.ReadSessionSaveData(uuid, 1)
        if err != nil {
            t.Fatal(err)
        }
        if saved.Seed != seed || saved.WaveIndex != 182 || saved.Money != 18200 {
            t.Fatalf("반복 %d: 최종 저장값 오류: seed=%q, wave=%d, money=%d", i, saved.Seed, saved.WaveIndex, saved.Money)
        }
    }
    t.Log("성공: 동시 저장 30회 모두 최종 182층 유지")
}
