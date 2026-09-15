package world

import (
	"encoding/json"
	"os"
)

type Location struct {
	Name 		string					`json:"name"`
	Description string					`json:"description"`
	Exits		map[string]string		`json:"exits"`
	Items		[]string				`json:"items"`
	NPCs		[]string				`json:"npcs"`
}

type Item struct {
	Name			string	`json:"name"`
	Description		string	`json:"description"`
	Obtainable		bool	`json:"obtainable"`
}

type NPC struct {
	Name		string		`json:"name"`
	Description	string		`json:"description"`
	Role		string		`json:"role"`
	Dialogue	[]string	`json:"dialogue"`
	HP			int			`json:"hp"`
	Damage		int			`json:"damage"`
	Drops		[]string	`json:"drops"`
	Quests		[]string	`json:"quests"`
}

type Quest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Giver       string `json:"giver"`
	Type        string `json:"type"`   // "fetch" or "kill"
	Target      string `json:"target"` // item id for fetch, npc id for kill
	Proof       string `json:"proof"`  // item the kill quest requires you to bring back
	RewardHP    int    `json:"reward_hp"`
}

type World struct {
	Locations map[string]*Location `json:"locations"`
	Items     map[string]*Item     `json:"items"`
	NPCs      map[string]*NPC      `json:"npcs"`
	Quests    map[string]*Quest    `json:"quests"`
}

// Load reads a world JSON and parses it.
func Load(path string) (*World, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var w World
	if err := json.Unmarshal(data, &w); err != nil { // &w so it can be filled in
		return nil, err
	}
	return &w, nil
}