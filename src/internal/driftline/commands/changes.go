package commands

import "github.com/y-writings/driftline/src/internal/driftline"

func sortedChanges(changes []driftline.Change) []driftline.Change {
	return driftline.SortedChanges(changes)
}
