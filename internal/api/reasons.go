package api

import (
	"maps"
	"slices"

	"github.com/Soo-de/suiro/internal/model"
)

// Reasons returns every code of model.Reasons once, sorted by code, with its
// level and scope.
func Reasons() ReasonList {
	table := model.Reasons()
	items := make([]Reason, 0, len(table))
	for _, code := range slices.Sorted(maps.Keys(table)) {
		items = append(items, Reason{Code: code, Level: table[code].Level, Scope: table[code].Scope})
	}
	return ReasonList{Items: items}
}
