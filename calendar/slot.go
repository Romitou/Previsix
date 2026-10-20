// Package calendar defines the operating slots shared by the calendar sources.
package calendar

import (
	"context"
	"time"
)

// Slot is an opening period of the park. A day usually has a single slot, but
// event nights (Peur sur le Parc, ...) are separate slots on the same day.
type Slot struct {
	// Paris day of the opening, YYYY-MM-DD.
	Date  string
	Open  time.Time
	Close time.Time
	// Event is true for separately ticketed slots, such as Halloween nights.
	Event bool
}

// Source fetches the upcoming operating slots of the park.
type Source interface {
	Name() string
	Slots(ctx context.Context) ([]Slot, error)
}
