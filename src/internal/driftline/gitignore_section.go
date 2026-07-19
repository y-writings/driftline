package driftline

import (
	"bytes"
	"fmt"
	"strings"
)

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

type gitIgnoreTransform struct {
	Changed      bool
	Status       Status
	Reason       string
	DesiredBytes []byte
}

type gitIgnoreSection struct {
	found     bool
	start     int
	end       int
	delimiter []byte
}

type gitIgnoreMarkerLine struct {
	start     int
	end       int
	delimiter []byte
}

func transformGitIgnoreSection(current []byte, targetMissing bool, repository string, config *ContractGitIgnore) (gitIgnoreTransform, error) {
	section, err := parseGitIgnoreSection(current)
	if err != nil {
		return gitIgnoreTransform{}, err
	}

	if config == nil || len(config.Entries) == 0 {
		if !section.found {
			return gitIgnoreTransform{DesiredBytes: current}, nil
		}
		desired := replaceGitIgnoreSpan(current, section.start, section.end, nil)
		return gitIgnoreTransform{
			Changed:      true,
			Status:       StatusRemove,
			Reason:       "generated section is no longer declared",
			DesiredBytes: desired,
		}, nil
	}

	delimiter := []byte("\n")
	if section.found {
		delimiter = section.delimiter
	} else if !targetMissing {
		delimiter = firstGitIgnoreDelimiter(current)
	}
	generated := renderGitIgnoreSection(repository, config.Entries, delimiter)

	if targetMissing {
		return gitIgnoreTransform{
			Changed:      true,
			Status:       StatusAdd,
			Reason:       "generated section is missing",
			DesiredBytes: generated,
		}, nil
	}
	if section.found {
		desired := replaceGitIgnoreSpan(current, section.start, section.end, generated)
		if bytes.Equal(current, desired) {
			return gitIgnoreTransform{DesiredBytes: current}, nil
		}
		return gitIgnoreTransform{
			Changed:      true,
			Status:       StatusUpdate,
			Reason:       "generated section differs",
			DesiredBytes: desired,
		}, nil
	}

	desired := appendGitIgnoreSection(current, generated, delimiter)
	return gitIgnoreTransform{
		Changed:      true,
		Status:       StatusAdd,
		Reason:       "generated section is missing",
		DesiredBytes: desired,
	}, nil
}

func parseGitIgnoreSection(current []byte) (gitIgnoreSection, error) {
	var starts []gitIgnoreMarkerLine
	var ends []gitIgnoreMarkerLine
	for lineStart := 0; lineStart < len(current); {
		lineEnd := len(current)
		spanEnd := len(current)
		var delimiter []byte
		if relativeLF := bytes.IndexByte(current[lineStart:], '\n'); relativeLF >= 0 {
			lf := lineStart + relativeLF
			lineEnd = lf
			delimiterStart := lf
			if lf > lineStart && current[lf-1] == '\r' {
				lineEnd--
				delimiterStart--
			}
			spanEnd = lf + 1
			delimiter = current[delimiterStart:spanEnd]
		}

		line := current[lineStart:lineEnd]
		marker := gitIgnoreMarkerLine{start: lineStart, end: spanEnd, delimiter: delimiter}
		if isGitIgnoreStartMarker(line) {
			starts = append(starts, marker)
		}
		if bytes.Equal(line, []byte(gitIgnoreEndMarker)) {
			ends = append(ends, marker)
		}

		if spanEnd == len(current) {
			break
		}
		lineStart = spanEnd
	}

	const errorPrefix = "invalid driftline section in .gitignore: "
	if len(starts) == 0 && len(ends) == 0 {
		return gitIgnoreSection{}, nil
	}
	if len(starts) == 1 && len(ends) == 0 {
		return gitIgnoreSection{}, fmt.Errorf("%sstart marker has no matching end marker", errorPrefix)
	}
	if len(starts) == 0 && len(ends) == 1 {
		return gitIgnoreSection{}, fmt.Errorf("%send marker has no matching start marker", errorPrefix)
	}
	if len(starts) != 1 || len(ends) != 1 {
		return gitIgnoreSection{}, fmt.Errorf("%sfound %d start markers and %d end markers; expected exactly one of each", errorPrefix, len(starts), len(ends))
	}
	if ends[0].start < starts[0].start {
		return gitIgnoreSection{}, fmt.Errorf("%send marker appears before start marker", errorPrefix)
	}

	return gitIgnoreSection{
		found:     true,
		start:     starts[0].start,
		end:       ends[0].end,
		delimiter: starts[0].delimiter,
	}, nil
}

func renderGitIgnoreSection(repository string, entries []string, delimiter []byte) []byte {
	var rendered bytes.Buffer
	writeLine := func(line string) {
		rendered.WriteString(line)
		rendered.Write(delimiter)
	}

	writeLine(gitIgnoreStartPrefix + repository + gitIgnoreContractTail)
	writeLine(gitIgnoreWarning)
	for _, entry := range entries {
		writeLine(entry)
	}
	writeLine(gitIgnoreEndMarker)
	return rendered.Bytes()
}

func firstGitIgnoreDelimiter(current []byte) []byte {
	lf := bytes.IndexByte(current, '\n')
	if lf > 0 && current[lf-1] == '\r' {
		return []byte("\r\n")
	}
	return []byte("\n")
}

func appendGitIgnoreSection(current []byte, generated []byte, delimiter []byte) []byte {
	desired := append([]byte(nil), current...)
	if len(current) > 0 && !endsWithEmptyGitIgnoreLine(current) {
		if current[len(current)-1] != '\n' {
			desired = append(desired, delimiter...)
		}
		desired = append(desired, delimiter...)
	}
	return append(desired, generated...)
}

func endsWithEmptyGitIgnoreLine(current []byte) bool {
	if len(current) == 0 || current[len(current)-1] != '\n' {
		return false
	}
	delimiterStart := len(current) - 1
	if delimiterStart > 0 && current[delimiterStart-1] == '\r' {
		delimiterStart--
	}
	contentStart := bytes.LastIndexByte(current[:delimiterStart], '\n') + 1
	return contentStart == delimiterStart
}

func replaceGitIgnoreSpan(current []byte, start int, end int, replacement []byte) []byte {
	desired := make([]byte, 0, len(current)-(end-start)+len(replacement))
	desired = append(desired, current[:start]...)
	desired = append(desired, replacement...)
	return append(desired, current[end:]...)
}
