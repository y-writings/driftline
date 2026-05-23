package driftline

type Manifest struct {
	Version   int            `yaml:"version"`
	GitIgnore []string       `yaml:"gitignore,omitempty"`
	File      []ManifestFile `yaml:"files"`
}

type ManifestFile struct {
	ID          string `yaml:"id"`
	Source      string `yaml:"source"`
	Target      string `yaml:"target"`
	IfNotExists bool   `yaml:"if_not_exists,omitempty"`
}

type LockFile struct {
	Repository string              `yaml:"repository,omitempty"`
	Ref        string              `yaml:"ref,omitempty"`
	Files      map[string]LockItem `yaml:"files,omitempty"`
}

type LockItem struct {
	Target       string `yaml:"target"`
	SourceSHA256 string `yaml:"source_sha256"`
}

type Status string

const (
	StatusSynced   Status = "synced"
	StatusAdd      Status = "add"
	StatusUpdate   Status = "update"
	StatusPrune    Status = "prune"
	StatusConflict Status = "conflict"
)

type Change struct {
	ID          string
	SourcePath  string
	TargetPath  string
	SourceHash  string
	CurrentHash string
	LockedHash  string
	Status      Status
	Reason      string
}
