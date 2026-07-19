package driftline

import "sort"

type FileMode string

const (
	ModeManaged  FileMode = "managed"
	ModeTemplate FileMode = "template"
)

type Contract struct {
	Version   int                                `toml:"version"`
	GitIgnore *ContractGitIgnore                 `toml:"gitignore"`
	Files     map[string]map[string]ContractFile `toml:"files"`
}

type ContractGitIgnore struct {
	Entries []string `toml:"entries"`
}

type ContractFile struct {
	Path string   `toml:"path"`
	Mode FileMode `toml:"mode"`
}

type SyncManifest struct {
	Version int                          `toml:"version"`
	Source  SyncSource                   `toml:"source"`
	Files   map[string]map[string]string `toml:"files"`
}

type SyncSource struct {
	Repository string `toml:"repository"`
	Ref        string `toml:"ref"`
}

type ContractEntry struct {
	Group string
	File  string
	Key   string
	Path  string
	Mode  FileMode
}

type SyncEntry struct {
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
	StatusSyncManifestAdd    Status = "sync-manifest-add"
	StatusSyncManifestRemove Status = "sync-manifest-remove"
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
