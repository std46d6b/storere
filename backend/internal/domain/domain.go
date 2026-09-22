package domain

import "errors"

type State string

const (
	StateActive                 State = "active"
	StateTemporarilyUnavailable State = "temporarily_unavailable"
	StateArchived               State = "archived"
)

type Box struct {
	CurrentLocationID   string
	TemporaryLocationID string
}

func (b Box) EffectiveLocationID() string {
	if b.TemporaryLocationID != "" {
		return b.TemporaryLocationID
	}
	return b.CurrentLocationID
}

type Item struct {
	Name       string
	State      State
	MediaCount int
}

func (i Item) Validate() error {
	if i.Name == "" {
		return errors.New("item name is required")
	}
	if i.State == StateActive && i.MediaCount < 1 {
		return errors.New("an active item requires at least one photo")
	}
	return nil
}
