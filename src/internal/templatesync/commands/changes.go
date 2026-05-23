package commands

import (
	"sort"

	"github.com/y-writings/templates/src/internal/templatesync"
)

func sortedChanges(changes []templatesync.Change) []templatesync.Change {
	out := append([]templatesync.Change(nil), changes...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status == out[j].Status {
			return out[i].ID < out[j].ID
		}
		return out[i].Status < out[j].Status
	})
	return out
}
