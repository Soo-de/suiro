package sets_test

import (
	"testing"

	"github.com/Soo-de/suiro/internal/core/sets"
	"github.com/Soo-de/suiro/internal/core/sets/settest"
)

func TestIntervalSpaceLaws(t *testing.T) {
	settest.Laws(t, sets.NewIntervalSpace())
}
