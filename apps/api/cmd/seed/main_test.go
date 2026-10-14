package main

import "testing"

func TestShouldDeleteStaleSeedEmailsDefaultsToFalse(t *testing.T) {
	t.Setenv("SEED_DELETE_STALE_EMAILS", "")

	if shouldDeleteStaleSeedEmails() {
		t.Fatal("expected stale seed email deletion to be disabled by default")
	}
}

func TestShouldDeleteStaleSeedEmailsRequiresExplicitTrue(t *testing.T) {
	t.Setenv("SEED_DELETE_STALE_EMAILS", "true")

	if !shouldDeleteStaleSeedEmails() {
		t.Fatal("expected stale seed email deletion to be enabled when explicitly requested")
	}
}
