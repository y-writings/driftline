package driftline

import "strings"

const (
	GitIgnorePath         = ".gitignore"
	gitIgnoreStartPrefix  = "# start driftline from "
	gitIgnoreContractTail = "/" + ContractPath
	gitIgnoreWarning      = "# DO NOT EDIT: this section is managed automatically by driftline."
	gitIgnoreEndMarker    = "# end driftline"
)

func gitIgnoreStartRepository(line []byte) (string, bool) {
	marker := string(line)
	repositoryWithTail, ok := strings.CutPrefix(marker, gitIgnoreStartPrefix)
	if !ok {
		return "", false
	}
	repository, ok := strings.CutSuffix(repositoryWithTail, gitIgnoreContractTail)
	if !ok || ValidateRepository(repository) != nil {
		return "", false
	}
	return repository, true
}

func isGitIgnoreStartMarker(line []byte) bool {
	_, ok := gitIgnoreStartRepository(line)
	return ok
}
