package main

import "testing"

func TestRateChangerWithoutFirestoreProjectIsUnavailable(t *testing.T) {
	t.Setenv("FIREBASE_PROJECT_ID", "")

	svc := newRateChanger()

	if svc.Store != nil {
		t.Fatal("expected no Firestore store without FIREBASE_PROJECT_ID")
	}
	if ok, err := svc.Available(t.Context(), 8); ok || err != nil {
		t.Fatalf("expected unavailable without error, got %v, %v", ok, err)
	}
}
