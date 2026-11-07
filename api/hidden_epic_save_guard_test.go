package api

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/pagefaultgames/rogueserver/defs"
)

func hiddenEpicTestSave() defs.SystemSaveData {
	return defs.SystemSaveData{
		RoguePoints: json.RawMessage(`0`), BankMoney: json.RawMessage(`0`),
		BankRoguePoints: json.RawMessage(`0`),
		StorageItems:    json.RawMessage(`[]`), QuestList: json.RawMessage(`[]`),
	}
}

func TestHiddenEpicSaveGuardAllowsEmptyNewAccount(t *testing.T) {
	if err := validateHiddenEpicSystemSave(hiddenEpicTestSave(), defs.SystemSaveData{}, false); err != nil {
		t.Fatal(err)
	}
	if err := validateHiddenEpicSystemSave(hiddenEpicTestSave(), defs.SystemSaveData{}, true); err != nil {
		t.Fatal(err)
	}
}
func TestHiddenEpicSaveGuardRejectsMissingEssentialFields(t *testing.T) {
	cases := map[string]func(*defs.SystemSaveData){
		"roguePoints":     func(s *defs.SystemSaveData) { s.RoguePoints = nil },
		"bankMoney":       func(s *defs.SystemSaveData) { s.BankMoney = nil },
		"bankRoguePoints": func(s *defs.SystemSaveData) { s.BankRoguePoints = nil },
		"storageItems":    func(s *defs.SystemSaveData) { s.StorageItems = nil },
		"questList":       func(s *defs.SystemSaveData) { s.QuestList = nil },
	}
	for name, modify := range cases {
		t.Run(name, func(t *testing.T) {
			s := hiddenEpicTestSave()
			modify(&s)
			if err := validateHiddenEpicSystemSave(s, defs.SystemSaveData{}, true); !errors.Is(err, ErrMissingHiddenEpicSaveData) {
				t.Fatalf("expected guard error, got %v", err)
			}
		})
	}
}
func TestHiddenEpicSaveGuardRejectsInvalidTypes(t *testing.T) {
	cases := []struct {
		name   string
		value  json.RawMessage
		assign func(*defs.SystemSaveData, json.RawMessage)
	}{
		{"negativeRP", json.RawMessage(`-1`), func(s *defs.SystemSaveData, v json.RawMessage) { s.RoguePoints = v }},
		{"decimalRP", json.RawMessage(`1.5`), func(s *defs.SystemSaveData, v json.RawMessage) { s.RoguePoints = v }},
		{"nullStorage", json.RawMessage(`null`), func(s *defs.SystemSaveData, v json.RawMessage) { s.StorageItems = v }},
		{"objectQuests", json.RawMessage(`{}`), func(s *defs.SystemSaveData, v json.RawMessage) { s.QuestList = v }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := hiddenEpicTestSave()
			tc.assign(&s, tc.value)
			if err := validateHiddenEpicSystemSave(s, defs.SystemSaveData{}, false); !errors.Is(err, ErrMissingHiddenEpicSaveData) {
				t.Fatalf("expected guard error, got %v", err)
			}
		})
	}
}
func TestHiddenEpicSaveGuardPreservesExistingExtensions(t *testing.T) {
	old := hiddenEpicTestSave()
	old.CompanionData = json.RawMessage(`{}`)
	if err := validateHiddenEpicSystemSave(hiddenEpicTestSave(), old, true); !errors.Is(err, ErrMissingHiddenEpicSaveData) {
		t.Fatalf("expected guard error, got %v", err)
	}
	current := hiddenEpicTestSave()
	current.CompanionData = json.RawMessage(`{}`)
	if err := validateHiddenEpicSystemSave(current, old, true); err != nil {
		t.Fatal(err)
	}
	old.HiddenEpicExtra = map[string]json.RawMessage{"futureAccountField": json.RawMessage(`42`)}
	if err := validateHiddenEpicSystemSave(current, old, true); !errors.Is(err, ErrMissingHiddenEpicSaveData) {
		t.Fatalf("expected future field guard error, got %v", err)
	}
}
