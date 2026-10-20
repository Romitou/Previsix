package forecast

import "sync/atomic"

var current atomic.Pointer[Model]

func init() {
	current.Store(PriorModel())
}

// Current returns the model used to make forecasts.
func Current() *Model {
	return current.Load()
}

// SetCurrent replaces the model used to make forecasts.
func SetCurrent(model *Model) {
	current.Store(model)
}
