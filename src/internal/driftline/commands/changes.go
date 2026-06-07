package commands

import (
	"sort"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func sortedChanges(changes []driftline.Change) []driftline.Change {
	out := append([]driftline.Change(nil), changes...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return out[i].Status < out[j].Status
		}
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Target < out[j].Target
	})
	return out
}
