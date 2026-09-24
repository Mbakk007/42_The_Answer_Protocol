package world

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Location struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Exits       map[string]string `json:"exits"`
	Items       []string          `json:"items"`
	NPCs        []string          `json:"npcs"`
}

type Item struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Obtainable  bool   `json:"obtainable"`
}

type NPC struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Role        string   `json:"role"`
	Dialogue    []string `json:"dialogue"`
	HP          int      `json:"hp"`
	CurrentHP   int      `json:"-"`
	Damage      int      `json:"damage"`
	Drops       []string `json:"drops"`
	Quests      []string `json:"quests"`
}

type Quest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Giver       string `json:"giver"`
	Type        string `json:"type"`
	Target      string `json:"target"`
	Proof       string `json:"proof"`
	RewardHP    int    `json:"reward_hp"`
}

type World struct {
	Locations map[string]*Location `json:"locations"`
	Items     map[string]*Item     `json:"items"`
	NPCs      map[string]*NPC      `json:"npcs"`
	Quests    map[string]*Quest    `json:"quests"`
}

// Load reads a world JSON, parses it and validates its references.
func Load(path string) (*World, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var w World
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}

	for id, loc := range w.Locations {
		loc.ID = id
		if loc.Items == nil {
			loc.Items = []string{}
		}
		if loc.NPCs == nil {
			loc.NPCs = []string{}
		}
	}
	for _, npc := range w.NPCs {
		npc.CurrentHP = npc.HP
	}

	if err := w.Validate(); err != nil {
		return nil, err
	}

	return &w, nil
}

// Validate reports every dangling reference in the world at once.
func (w *World) Validate() error {
	var problems []string

	for id, loc := range w.Locations {
		for dir, dest := range loc.Exits {
			if _, ok := w.Locations[dest]; !ok {
				problems = append(problems, fmt.Sprintf("%s: exit %s -> unknown room %s", id, dir, dest))
			}
		}
		for _, it := range loc.Items {
			if _, ok := w.Items[it]; !ok {
				problems = append(problems, fmt.Sprintf("%s: unknown item %s", id, it))
			}
		}
		for _, n := range loc.NPCs {
			if _, ok := w.NPCs[n]; !ok {
				problems = append(problems, fmt.Sprintf("%s: unknown npc %s", id, n))
			}
		}
	}

	for id, npc := range w.NPCs {
		for _, d := range npc.Drops {
			if _, ok := w.Items[d]; !ok {
				problems = append(problems, fmt.Sprintf("%s: drops unknown item %s", id, d))
			}
		}
		for _, q := range npc.Quests {
			if _, ok := w.Quests[q]; !ok {
				problems = append(problems, fmt.Sprintf("%s: gives unknown quest %s", id, q))
			}
		}
	}

	for id, q := range w.Quests {
		if _, ok := w.NPCs[q.Giver]; !ok {
			problems = append(problems, fmt.Sprintf("%s: unknown giver %s", id, q.Giver))
		}
		_, isItem := w.Items[q.Target]
		_, isNPC := w.NPCs[q.Target]
		if !isItem && !isNPC {
			problems = append(problems, fmt.Sprintf("%s: unknown target %s", id, q.Target))
		}
		if q.Proof != "" {
			if _, ok := w.Items[q.Proof]; !ok {
				problems = append(problems, fmt.Sprintf("%s: unknown proof %s", id, q.Proof))
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid world:\n%s", strings.Join(problems, "\n"))
	}
	return nil
}
