package scenario

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/emms-garcia/shipq/internal/domain/identity"
)

type Step struct {
	At          time.Time
	Changes     map[string]any
	Expected    map[string]any
	PRPages     []string
	ReviewPages []string
}

type rawStep struct {
	At          string         `json:"at"`
	Changes     map[string]any `json:"changes"`
	Expected    map[string]any `json:"expected"`
	PRPages     []string       `json:"pr_pages"`
	ReviewPages []string       `json:"review_pages"`
}

type rawScenario struct {
	ID    string    `json:"id"`
	Steps []rawStep `json:"steps"`
}

type document struct {
	FormatVersion int           `json:"format_version"`
	Scenarios     []rawScenario `json:"scenarios"`
}

type Driver struct {
	mu         sync.Mutex
	document   document
	positions  map[string]int
	now        time.Time
	initial    time.Time
	idSequence uint64
}

func Load(path string) (*Driver, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var content document
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&content); err != nil {
		return nil, fmt.Errorf("decode scenario fixture: %w", err)
	}
	if content.FormatVersion != 1 || len(content.Scenarios) == 0 {
		return nil, errors.New("unsupported or empty scenario fixture")
	}
	initial := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	positions := make(map[string]int, len(content.Scenarios))
	for _, scenario := range content.Scenarios {
		if scenario.ID == "" || len(scenario.Steps) == 0 {
			return nil, errors.New("scenario IDs and steps must be nonempty")
		}
		if _, duplicate := positions[scenario.ID]; duplicate {
			return nil, errors.New("scenario IDs must be unique")
		}
		positions[scenario.ID] = 0
	}
	return &Driver{document: content, positions: positions, now: initial, initial: initial}, nil
}

func (d *Driver) FormatVersion() int { return d.document.FormatVersion }

func (d *Driver) ScenarioIDs() []string {
	result := make([]string, 0, len(d.document.Scenarios))
	for _, scenario := range d.document.Scenarios {
		result = append(result, scenario.ID)
	}
	slices.Sort(result)
	return result
}

func (d *Driver) Advance(id string) (Step, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, scenario := range d.document.Scenarios {
		if scenario.ID != id {
			continue
		}
		position := d.positions[id]
		if position >= len(scenario.Steps) {
			return Step{}, errors.New("scenario is exhausted")
		}
		raw := scenario.Steps[position]
		instant, err := time.Parse(time.RFC3339Nano, raw.At)
		if err != nil || !instant.Equal(instant.UTC()) {
			return Step{}, errors.New("scenario time must be UTC RFC 3339")
		}
		d.positions[id] = position + 1
		d.now = instant
		return Step{At: instant, Changes: raw.Changes, Expected: raw.Expected, PRPages: raw.PRPages, ReviewPages: raw.ReviewPages}, nil
	}
	return Step{}, errors.New("unknown scenario")
}

func (d *Driver) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for id := range d.positions {
		d.positions[id] = 0
	}
	d.now = d.initial
	d.idSequence = 0
}

func (d *Driver) Now() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.now
}

func (d *Driver) NewID() (identity.ID, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.idSequence++
	seed := sha256.Sum256([]byte(fmt.Sprintf("shipq-fixture-%d", d.idSequence)))
	seed[6] = (seed[6] & 0x0f) | 0x40
	seed[8] = (seed[8] & 0x3f) | 0x80
	var result identity.ID
	copy(result[:], seed[:16])
	return result, nil
}
