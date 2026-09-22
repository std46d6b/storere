package domain

import "testing"

func TestEffectiveLocationPrefersTemporaryLocation(t *testing.T) {
	box := Box{CurrentLocationID: "balcony", TemporaryLocationID: "hall"}
	if got := box.EffectiveLocationID(); got != "hall" {
		t.Fatalf("expected temporary location hall, got %q", got)
	}
}

func TestActiveItemRequiresPhoto(t *testing.T) {
	item := Item{Name: "Ski gloves", State: StateActive}
	if err := item.Validate(); err == nil {
		t.Fatal("expected active item without photo to be invalid")
	}
	item.MediaCount = 1
	if err := item.Validate(); err != nil {
		t.Fatalf("expected active item with photo to be valid: %v", err)
	}
}
