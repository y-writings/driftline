package driftline

type SourceManifest struct {
	Version   int                  `yaml:"version"`
	GitIgnore []string             `yaml:"gitignore,omitempty"`
	Files     []SourceManifestFile `yaml:"files"`
}

type SourceManifestFile struct {
	ID          string `yaml:"id"`
	Source      string `yaml:"source"`
	Target      string `yaml:"target"`
	IfNotExists bool   `yaml:"if_not_exists,omitempty"`
}

type TargetConfig struct {
	Version int                `yaml:"version"`
	Source  TargetSource       `yaml:"source"`
	Files   []TargetConfigFile `yaml:"files"`
}

type TargetSource struct {
	Repository string `yaml:"repository"`
	Ref        string `yaml:"ref"`
}

type TargetConfigFile struct {
	ID          string `yaml:"id"`
	Target      string `yaml:"target,omitempty"`
	IfNotExists *bool  `yaml:"if_not_exists,omitempty"`
}

type LockFile struct {
	Version    int        `yaml:"version"`
	Repository string     `yaml:"repository"`
	Ref        string     `yaml:"ref"`
	Commit     string     `yaml:"commit"`
	Files      []LockItem `yaml:"files"`
}

type LockItem struct {
	ID           string `yaml:"id"`
	Target       string `yaml:"target"`
	SourceSHA256 string `yaml:"source_sha256"`
	TargetSHA256 string `yaml:"target_sha256"`
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
	ID              string
	Target          string
	SourcePath      string
	TargetPath      string
	SourceBytes     []byte
	SourceHash      string
	CurrentHash     string
	LockedSource    string
	LockedTarget    string
	Status          Status
	Reason          string
	WritesTarget    bool
	PreservesTarget bool
}
