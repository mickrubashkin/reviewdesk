package main

import "testing"

func TestBuildPlan(t *testing.T) {
	emails := []Email{
		{ID: "old-unsent", Stage: "approved", SortOrder: 301, Language: "de", Variant: "old"},
		{ID: "new-de", Stage: "approved", SortOrder: 301, Language: "de", Variant: "new"},
		{ID: "old-unsent-alone", Stage: "lost", SortOrder: 1101, Language: "de", Variant: "old"},
		{ID: "old-only", Stage: "approved", SortOrder: 302, Language: "de", Variant: "old", Sends: 3},
		{ID: "old-with-new", Stage: "approved", SortOrder: 301, Language: "br", Variant: "old", Sends: 22},
		{ID: "new-br", Stage: "approved", SortOrder: 301, Language: "br", Variant: "new"},
		{ID: "old-marked", Stage: "approved", SortOrder: 302, Language: "br", Variant: "old", Sends: 5, Live: true},
		// A new version of another adaptation does not count for the slot.
		{ID: "old-adapted", Stage: "approved", SortOrder: 303, Language: "es", Variant: "old", Sends: 1},
		{ID: "new-latam", Stage: "approved", SortOrder: 303, Language: "es", Variant: "new", AdaptationKey: "latam"},
	}
	want := map[string]Action{
		"old-unsent":       ActionArchive,
		"old-unsent-alone": ActionCheck,
		"old-only":         ActionPromote,
		"old-with-new":     ActionStillInRobot,
		"old-adapted":      ActionPromote,
	}
	steps := BuildPlan(emails)
	if len(steps) != len(want) {
		t.Fatalf("expected %d steps, got %d", len(want), len(steps))
	}
	for _, step := range steps {
		if step.Action != want[step.Email.ID] {
			t.Errorf("%s: expected %s, got %s", step.Email.ID, want[step.Email.ID], step.Action)
		}
	}
	for _, step := range steps {
		if step.Email.ID == "old-with-new" && (step.New == nil || step.New.ID != "new-br") {
			t.Fatalf("expected the still-in-robot step to point at the new version, got %+v", step.New)
		}
	}
}
