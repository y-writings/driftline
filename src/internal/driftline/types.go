package driftline

import "sort"

type FileMode string

const (
	ModeManaged  FileMode = "managed"
	ModeTemplate FileMode = "template"
)

type SourceManifest struct {
	Version int                                      `toml:"version"`
	Files   map[string]map[string]SourceManifestFile `toml:"files"`
}

type SourceManifestFile struct {
	Path string   `toml:"path"`
	Mode FileMode `toml:"mode"`
}

type TargetConfig struct {
	Version int                          `toml:"version"`
	Source  TargetSource                 `toml:"source"`
	Files   map[string]map[string]string `toml:"files"`
}

type TargetSource struct {
	Repository string `toml:"repository"`
	Ref        string `toml:"ref"`
}

type SourceEntry struct {
	Group string
	File  string
	Key   string
	Path  string
	Mode  FileMode
}

type TargetEntry struct {
	Group string
	File  string
	Key   string
	Path  string
}

type Status string

const (
	StatusSynced             Status = "synced"
	StatusAdd                Status = "add"
	StatusUpdate             Status = "update"
	StatusRemove             Status = "remove"
	StatusTargetConfigAdd    Status = "target-config-add"
	StatusTargetConfigRemove Status = "target-config-remove"
	StatusModeTransition     Status = "mode-transition"
	StatusConflict           Status = "conflict"
)

type Change struct {
	ID            string
	Target        string
	TargetPath    string
	SourcePath    string
	SourceBytes   []byte
	Status        Status
	Reason        string
	WritesTarget  bool
	DeletesTarget bool
	ForceAllowed  bool
}

func SortedChanges(changes []Change) []Change {
	out := append([]Change(nil), changes...)
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
