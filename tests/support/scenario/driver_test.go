package scenario

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDriverAdvancesExplicitStepsWithDeterministicClockAndIDs(t *testing.T) {
	driver, err := Load(filepath.Join("..", "..", "fixtures", "scenarios", "pr_management.json"))
	if err != nil {
		t.Fatal(err)
	}
	if driver.FormatVersion() != 1 || len(driver.ScenarioIDs()) != 3 {
		t.Fatal("scenario catalog did not load")
	}
	before := driver.Now()
	step, err := driver.Advance("draft-ready-and-second-approval")
	if err != nil {
		t.Fatal(err)
	}
	if !step.At.Equal(time.Date(2026, 1, 2, 13, 0, 0, 0, time.UTC)) || !driver.Now().Equal(step.At) || driver.Now().Equal(before) {
		t.Fatal("advance did not control the fake clock")
	}
	first, err := driver.NewID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := driver.NewID()
	if err != nil || first == second {
		t.Fatal("deterministic ID sequence repeated")
	}

	driver.Reset()
	if !driver.Now().Equal(before) {
		t.Fatal("reset did not restore the initial clock")
	}
}

func TestDriverRejectsUnknownAndExhaustedScenarios(t *testing.T) {
	driver, err := Load(filepath.Join("..", "..", "fixtures", "scenarios", "pr_management.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := driver.Advance("absent"); err == nil {
		t.Fatal("unknown scenario accepted")
	}
	for range 5 {
		if _, err := driver.Advance("draft-ready-and-second-approval"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := driver.Advance("draft-ready-and-second-approval"); err == nil {
		t.Fatal("exhausted scenario advanced")
	}
}
