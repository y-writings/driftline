package driftline

type PullConfig struct {
	Pull []PullSource `yaml:"pull"`
}

type PullSource struct {
	Repo    string   `yaml:"repo"`
	Exports []string `yaml:"exports"`
}

type ExportConfig map[string][]string

type LockFile struct {
	Repos []LockedRepo `yaml:"repos"`
}

type LockedRepo struct {
	Repo    string              `yaml:"repo"`
	From    string              `yaml:"from"`
	Exports map[string][]string `yaml:"exports"`
}
