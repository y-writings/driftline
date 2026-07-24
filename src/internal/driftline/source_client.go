package driftline

// SourceClient reads source repository references and files for planning and adoption.
type SourceClient interface {
	ResolveDefaultRef(repository string) (ref string, commit string, err error)
	ResolveRef(repository string, ref string) (commit string, err error)
	ReadFile(repository string, commit string, path string) ([]byte, error)
}
