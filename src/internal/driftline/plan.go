package driftline

type Options struct {
	ManifestPath string
	LockPath     string
	TargetDir    string
}

func DefaultOptions() Options {
	return Options{ManifestPath: ".driftline.yaml", LockPath: ".driftline-lock.yaml", TargetDir: "."}
}
