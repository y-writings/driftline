package driftline

type SourceManifest struct {
	Version   int                  `yaml:"version"`
	GitIgnore []string             `yaml:"gitignore,omitempty"`
	Files     []SourceManifestFile `yaml:"files"`
}

type SourceManifestFile struct {
	ID    string                    `yaml:"id"`
	Name  string                    `yaml:"name,omitempty"`
	Paths []SourceManifestPathEntry `yaml:"-"`
}

type SourceManifestPathEntry struct {
	ID   string
	Name string
	Path string
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
	ID            string            `yaml:"id"`
	PathOverrides map[string]string `yaml:"path_overrides,omitempty"`
	IfNotExists   bool              `yaml:"if_not_exists,omitempty"`
}

type LockFile struct {
	Version    int        `yaml:"version"`
	Repository string     `yaml:"repository"`
	Ref        string     `yaml:"ref"`
	Commit     string     `yaml:"commit"`
	Files      []LockItem `yaml:"files"`
}

type LockItem struct {
	ID         string `yaml:"id"`
	TargetPath string `yaml:"target_path"`
}

type Status string

const (
	StatusSynced Status = "synced"
	StatusAdd    Status = "add"
	StatusUpdate Status = "update"
	StatusPrune  Status = "prune"
)

type Change struct {
	ID           string
	Target       string
	TargetPath   string
	SourceBytes  []byte
	Status       Status
	Reason       string
	WritesTarget bool
}
