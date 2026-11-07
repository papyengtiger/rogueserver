/*
	Copyright (C) 2024 - 2025  Pagefault Games

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package defs

import "encoding/json"

const SessionSlotCount = 5

type SystemSaveData struct {
	TrainerId          int                `json:"trainerId"`
	SecretId           int                `json:"secretId"`
	Gender             int                `json:"gender"`
	DexData            DexData            `json:"dexData"`
	StarterData        StarterData        `json:"starterData"`
	StarterMoveData    StarterMoveData    `json:"starterMoveData"`    // Legacy
	StarterEggMoveData StarterEggMoveData `json:"starterEggMoveData"` // Legacy
	GameStats          GameStats          `json:"gameStats"`
	Unlocks            Unlocks            `json:"unlocks"`
	AchvUnlocks        AchvUnlocks        `json:"achvUnlocks"`
	VoucherUnlocks     VoucherUnlocks     `json:"voucherUnlocks"`
	VoucherCounts      VoucherCounts      `json:"voucherCounts"`
	Eggs               []EggData          `json:"eggs"`
	EggPity            []int              `json:"eggPity"`
	UnlockPity         []int              `json:"unlockPity"`
	GameVersion        string             `json:"gameVersion"`
	Timestamp          int                `json:"timestamp"`
	AppliedMigrators   map[string]int     `json:"appliedMigrators"`

	// Hidden Epic account-wide state. Raw JSON remains lossless for evolving
	// client-side object layouts, while gob stores the byte slices in MariaDB.
	ClearVoucherRewards json.RawMessage `json:"clearVoucherRewards"`
	CompanionData       json.RawMessage `json:"companionData"`
	RoguePoints         json.RawMessage `json:"roguePoints"`
	LastAttendanceDate  json.RawMessage `json:"lastAttendanceDate"`
	AttendanceCount     json.RawMessage `json:"attendanceCount"`
	AchvPointsGranted   json.RawMessage `json:"achvPointsGranted"`
	BankMoney           json.RawMessage `json:"bankMoney"`
	BankRoguePoints     json.RawMessage `json:"bankRoguePoints"`
	QuestList           json.RawMessage `json:"questList"`
	StorageItems        json.RawMessage `json:"storageItems"`
	PracticeDummyConfig json.RawMessage `json:"practiceDummyConfig"`
	PracticePresets     json.RawMessage `json:"practicePresets"`
	PendingRunItems     json.RawMessage `json:"pendingRunItems"`
	UsedCoupons         json.RawMessage `json:"usedCoupons"`
	BerryPlanterSlots   json.RawMessage `json:"berryPlanterSlots"`
	ResourceInventory   json.RawMessage `json:"resourceInventory"`

	// Preserve later client additions instead of discarding them during Decode.
	HiddenEpicExtra map[string]json.RawMessage `json:"-"`
}

type DexData map[int]DexEntry

type DexEntry struct {
	SeenAttr        interface{}                `json:"seenAttr"`   // integer or string
	CaughtAttr      interface{}                `json:"caughtAttr"` // integer or string
	NatureAttr      int                        `json:"natureAttr"`
	SeenCount       int                        `json:"seenCount"`
	CaughtCount     int                        `json:"caughtCount"`
	HatchedCount    int                        `json:"hatchedCount"`
	Ivs             []int                      `json:"ivs"`
	Ribbons         string                     `json:"ribbons"`
	HiddenEpicExtra map[string]json.RawMessage `json:"-"`
}

type StarterData map[int]StarterEntry

type StarterEntry struct {
	Moveset         interface{}                `json:"moveset"`
	EggMoves        int                        `json:"eggMoves"`
	CandyCount      int                        `json:"candyCount"`
	Friendship      int                        `json:"friendship"`
	AbilityAttr     int                        `json:"abilityAttr"`
	PassiveAttr     int                        `json:"passiveAttr"`
	ValueReduction  int                        `json:"valueReduction"`
	ClassicWinCount int                        `json:"classicWinCount"`
	HiddenEpicExtra map[string]json.RawMessage `json:"-"`
}

// preserveUnknownFields collects client extensions not recognized by the Go
// struct, allowing them to survive JSON -> gob -> JSON round-trips.
func preserveUnknownFields(input []byte, knownJSON []byte) (map[string]json.RawMessage, error) {
	var incoming map[string]json.RawMessage
	if err := json.Unmarshal(input, &incoming); err != nil {
		return nil, err
	}
	var known map[string]json.RawMessage
	if err := json.Unmarshal(knownJSON, &known); err != nil {
		return nil, err
	}
	for k := range known {
		delete(incoming, k)
	}
	return incoming, nil
}

func marshalWithUnknownFields(known any, extra map[string]json.RawMessage) ([]byte, error) {
	data, err := json.Marshal(known)
	if err != nil || len(extra) == 0 {
		return data, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for k, v := range extra {
		if _, exists := fields[k]; !exists {
			fields[k] = v
		}
	}
	return json.Marshal(fields)
}

func (s *SystemSaveData) UnmarshalJSON(data []byte) error {
	type plain SystemSaveData
	var tmp plain
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	known, err := json.Marshal(tmp)
	if err != nil {
		return err
	}
	tmp.HiddenEpicExtra, err = preserveUnknownFields(data, known)
	if err != nil {
		return err
	}
	*s = SystemSaveData(tmp)
	return nil
}

func (s SystemSaveData) MarshalJSON() ([]byte, error) {
	type plain SystemSaveData
	return marshalWithUnknownFields(plain(s), s.HiddenEpicExtra)
}

func (s *DexEntry) UnmarshalJSON(data []byte) error {
	type plain DexEntry
	var tmp plain
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	known, err := json.Marshal(tmp)
	if err != nil {
		return err
	}
	tmp.HiddenEpicExtra, err = preserveUnknownFields(data, known)
	if err != nil {
		return err
	}
	*s = DexEntry(tmp)
	return nil
}

func (s DexEntry) MarshalJSON() ([]byte, error) {
	type plain DexEntry
	return marshalWithUnknownFields(plain(s), s.HiddenEpicExtra)
}

func (s *StarterEntry) UnmarshalJSON(data []byte) error {
	type plain StarterEntry
	var tmp plain
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	known, err := json.Marshal(tmp)
	if err != nil {
		return err
	}
	tmp.HiddenEpicExtra, err = preserveUnknownFields(data, known)
	if err != nil {
		return err
	}
	*s = StarterEntry(tmp)
	return nil
}

func (s StarterEntry) MarshalJSON() ([]byte, error) {
	type plain StarterEntry
	return marshalWithUnknownFields(plain(s), s.HiddenEpicExtra)
}

type StarterMoveData map[int]interface{}

type StarterEggMoveData map[int]int

type GameStats interface{}

type Unlocks map[int]bool

type AchvUnlocks map[string]int

type VoucherUnlocks map[string]int

type VoucherCounts map[string]int

type EggData struct {
	Id                    int       `json:"id"`
	GachaType             GachaType `json:"gachaType"`
	HatchWaves            int       `json:"hatchWaves"`
	Timestamp             int       `json:"timestamp"`
	Tier                  int       `json:"tier"`
	SourceType            int       `json:"sourceType"`
	VariantTier           int       `json:"variantTier"`
	IsShiny               bool      `json:"isShiny"`
	Species               int       `json:"species"`
	EggMoveIndex          int       `json:"eggMoveIndex"`
	OverrideHiddenAbility bool      `json:"overrideHiddenAbility"`
}

type GachaType int

type SessionSaveData struct {
	Seed                     string                   `json:"seed"`
	PlayTime                 int                      `json:"playTime"`
	GameMode                 GameMode                 `json:"gameMode"`
	DailyConfig              DailyConfig              `json:"dailyConfig,omitempty"`
	Party                    []PokemonData            `json:"party"`
	EnemyParty               []PokemonData            `json:"enemyParty"`
	Modifiers                []PersistentModifierData `json:"modifiers"`
	EnemyModifiers           []PersistentModifierData `json:"enemyModifiers"`
	Arena                    ArenaData                `json:"arena"`
	PokeballCounts           PokeballCounts           `json:"pokeballCounts"`
	Money                    int                      `json:"money"`
	Score                    int                      `json:"score"`
	VictoryCount             int                      `json:"victoryCount"`
	FaintCount               int                      `json:"faintCount"`
	ReviveCount              int                      `json:"reviveCount"`
	WaveIndex                int                      `json:"waveIndex"`
	BattleType               BattleType               `json:"battleType"`
	Trainer                  TrainerData              `json:"trainer"`
	GameVersion              string                   `json:"gameVersion"`
	Timestamp                int                      `json:"timestamp"`
	Challenges               []ChallengeData          `json:"challenges"`
	MysteryEncounterType     MysteryEncounterType     `json:"mysteryEncounterType"`
	MysteryEncounterSaveData MysteryEncounterSaveData `json:"mysteryEncounterSaveData"`
	Name                     string                   `json:"name,omitempty"`
}

type ChallengeData struct {
	Id       int `json:"id"`
	Value    int `json:"value"`
	Severity int `json:"severity"`
}

type MysteryEncounterType int

type MysteryEncounterTier int

type SeenEncounterData struct {
	Type           MysteryEncounterType `json:"type"`
	Tier           MysteryEncounterTier `json:"tier"`
	WaveIndex      int                  `json:"waveIndex"`
	SelectedOption int                  `json:"selectedOption"`
}

type QueuedEncounter struct {
	Type         MysteryEncounterType `json:"type"`
	SpawnPercent int                  `json:"spawnPercent"`
}

type MysteryEncounterSaveData struct {
	EncounteredEvents    []SeenEncounterData `json:"encounteredEvents"`
	EncounterSpawnChance int                 `json:"encounterSpawnChance"`
	QueuedEncounters     []QueuedEncounter   `json:"queuedEncounters"`
}

type GameMode int

type DailyConfig interface{}

type PokemonData interface{}

type PersistentModifierData interface{}

type ArenaData interface{}

type PokeballCounts map[string]int

type BattleType int

type TrainerData interface{}

type SessionHistoryData struct {
	Seed        string                   `json:"seed"`
	PlayTime    int                      `json:"playTime"`
	Result      SessionHistoryResult     `json:"sessionHistoryResult"`
	GameMode    GameMode                 `json:"gameMode"`
	Party       []PokemonData            `json:"party"`
	Modifiers   []PersistentModifierData `json:"modifiers"`
	Money       int                      `json:"money"`
	Score       int                      `json:"score"`
	WaveIndex   int                      `json:"waveIndex"`
	BattleType  BattleType               `json:"battleType"`
	GameVersion string                   `json:"gameVersion"`
	Timestamp   int                      `json:"timestamp"`
}

type SessionHistoryResult int
