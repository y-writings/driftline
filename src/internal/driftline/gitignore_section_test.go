package driftline

import (
	"bytes"
	"strings"
	"testing"
)

const generatedGitIgnoreBlockLF = "# start driftline from new/repo/.driftline/contract.toml\n" +
	"# DO NOT EDIT: this section is managed automatically by driftline.\n" +
	".env\n" +
	"# end driftline\n"

func TestTransformGitIgnoreSectionCreatesMissingTarget(t *testing.T) {
	got, err := transformGitIgnoreSection(nil, true, "new/repo", &ContractGitIgnore{Entries: []string{".env"}})
	if err != nil {
		t.Fatalf("transform .gitignore: %v", err)
	}

	requireGitIgnoreTransform(t, got, true, StatusAdd, "generated section is missing", []byte(generatedGitIgnoreBlockLF))
}

func TestTransformGitIgnoreSectionAppendsWithoutDeduplicatingOutsideEntries(t *testing.T) {
	current := []byte("node_modules/\n.env\n")
	got, err := transformGitIgnoreSection(current, false, "new/repo", &ContractGitIgnore{Entries: []string{".env"}})
	if err != nil {
		t.Fatalf("transform .gitignore: %v", err)
	}

	want := append(append([]byte(nil), current...), []byte("\n"+generatedGitIgnoreBlockLF)...)
	requireGitIgnoreTransform(t, got, true, StatusUpdate, "generated section is missing", want)
}

func TestTransformGitIgnoreSectionAddsOnlyEnoughSeparatorLines(t *testing.T) {
	tests := []struct {
		name    string
		current string
		want    string
	}{
		{name: "empty existing file", current: "", want: "\n" + generatedGitIgnoreBlockLF},
		{name: "unterminated content", current: "cache", want: "cache\n\n" + generatedGitIgnoreBlockLF},
		{name: "terminated content", current: "cache\n", want: "cache\n\n" + generatedGitIgnoreBlockLF},
		{name: "whitespace-only line is not empty", current: "cache\n  \n", want: "cache\n  \n\n" + generatedGitIgnoreBlockLF},
		{name: "existing empty line", current: "cache\n\n", want: "cache\n\n" + generatedGitIgnoreBlockLF},
		{name: "existing empty lines are retained", current: "cache\n\n\n", want: "cache\n\n\n" + generatedGitIgnoreBlockLF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := transformGitIgnoreSection([]byte(tt.current), false, "new/repo", &ContractGitIgnore{Entries: []string{".env"}})
			if err != nil {
				t.Fatalf("transform .gitignore: %v", err)
			}
			requireGitIgnoreTransform(t, got, true, StatusUpdate, "generated section is missing", []byte(tt.want))
		})
	}
}

func TestTransformGitIgnoreSectionPreservesAuthoredEntryOrderAndBytes(t *testing.T) {
	config := &ContractGitIgnore{Entries: []string{
		".env",
		".env",
		"",
		"# authored comment",
		"  ",
		" *.log ",
		"!/dist/.gitkeep",
	}}
	got, err := transformGitIgnoreSection(nil, true, "new/repo", config)
	if err != nil {
		t.Fatalf("transform .gitignore: %v", err)
	}

	want := []byte("# start driftline from new/repo/.driftline/contract.toml\n" +
		"# DO NOT EDIT: this section is managed automatically by driftline.\n" +
		".env\n" +
		".env\n" +
		"\n" +
		"# authored comment\n" +
		"  \n" +
		" *.log \n" +
		"!/dist/.gitkeep\n" +
		"# end driftline\n")
	requireGitIgnoreTransform(t, got, true, StatusAdd, "generated section is missing", want)
}

func TestTransformGitIgnoreSectionReplacesSectionUsingStartDelimiter(t *testing.T) {
	current := []byte("keep\r\n\r\n" +
		"# start driftline from old/repo/.driftline/contract.toml\r\n" +
		"# stale warning\r\n" +
		"old-entry\r\n" +
		"# end driftline\r\n" +
		"after\n")
	got, err := transformGitIgnoreSection(current, false, "new/repo", &ContractGitIgnore{Entries: []string{".env"}})
	if err != nil {
		t.Fatalf("transform .gitignore: %v", err)
	}

	want := []byte("keep\r\n\r\n" +
		"# start driftline from new/repo/.driftline/contract.toml\r\n" +
		"# DO NOT EDIT: this section is managed automatically by driftline.\r\n" +
		".env\r\n" +
		"# end driftline\r\n" +
		"after\n")
	requireGitIgnoreTransform(t, got, true, StatusUpdate, "generated section differs", want)
}

func TestTransformGitIgnoreSectionRemovesOwnedSpanOnly(t *testing.T) {
	section := "# start driftline from old/repo/.driftline/contract.toml\n" +
		"# DO NOT EDIT: this section is managed automatically by driftline.\n" +
		"old-entry\n" +
		"# end driftline\n"
	tests := []struct {
		name    string
		current []byte
		config  *ContractGitIgnore
		want    []byte
	}{
		{
			name:    "retains preceding separator and following bytes",
			current: append([]byte("keep\n\n"+section), []byte{'a', 'f', 't', 'e', 'r', 0xff, '\n'}...),
			config:  nil,
			want:    append([]byte("keep\n\n"), []byte{'a', 'f', 't', 'e', 'r', 0xff, '\n'}...),
		},
		{
			name:    "block-only file may become empty",
			current: []byte(section),
			config:  &ContractGitIgnore{Entries: []string{}},
			want:    []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := transformGitIgnoreSection(tt.current, false, "new/repo", tt.config)
			if err != nil {
				t.Fatalf("transform .gitignore: %v", err)
			}
			requireGitIgnoreTransform(t, got, true, StatusRemove, "generated section is no longer declared", tt.want)
		})
	}
}

func TestTransformGitIgnoreSectionUsesFirstDelimiterWhenAppendingMixedFile(t *testing.T) {
	current := []byte("first\r\nsecond\n")
	got, err := transformGitIgnoreSection(current, false, "new/repo", &ContractGitIgnore{Entries: []string{".env"}})
	if err != nil {
		t.Fatalf("transform .gitignore: %v", err)
	}

	want := []byte("first\r\nsecond\n\r\n" +
		"# start driftline from new/repo/.driftline/contract.toml\r\n" +
		"# DO NOT EDIT: this section is managed automatically by driftline.\r\n" +
		".env\r\n" +
		"# end driftline\r\n")
	requireGitIgnoreTransform(t, got, true, StatusUpdate, "generated section is missing", want)
}

func TestTransformGitIgnoreSectionReplacesFinalBlockWithoutDelimiter(t *testing.T) {
	current := []byte("# start driftline from old/repo/.driftline/contract.toml\n" +
		"# stale warning\n" +
		"old-entry\n" +
		"# end driftline")
	got, err := transformGitIgnoreSection(current, false, "new/repo", &ContractGitIgnore{Entries: []string{".env"}})
	if err != nil {
		t.Fatalf("transform .gitignore: %v", err)
	}

	requireGitIgnoreTransform(t, got, true, StatusUpdate, "generated section differs", []byte(generatedGitIgnoreBlockLF))
}

func TestTransformGitIgnoreSectionNoOps(t *testing.T) {
	tests := []struct {
		name    string
		current []byte
		config  *ContractGitIgnore
	}{
		{
			name:    "matching generated bytes",
			current: []byte(generatedGitIgnoreBlockLF),
			config:  &ContractGitIgnore{Entries: []string{".env"}},
		},
		{name: "absent config without markers", current: []byte("local\n"), config: nil},
		{name: "empty config without markers", current: []byte("local\n"), config: &ContractGitIgnore{Entries: []string{}}},
		{name: "lone CR is ordinary content", current: []byte("# end driftline\r"), config: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := transformGitIgnoreSection(tt.current, false, "new/repo", tt.config)
			if err != nil {
				t.Fatalf("transform .gitignore: %v", err)
			}
			requireGitIgnoreTransform(t, got, false, "", "", tt.current)
		})
	}
}

func TestTransformGitIgnoreSectionPreservesInvalidUTF8(t *testing.T) {
	current := []byte{0xff, 'x', '\n'}
	got, err := transformGitIgnoreSection(current, false, "new/repo", &ContractGitIgnore{Entries: []string{".env"}})
	if err != nil {
		t.Fatalf("transform .gitignore: %v", err)
	}

	want := append(append([]byte(nil), current...), []byte("\n"+generatedGitIgnoreBlockLF)...)
	requireGitIgnoreTransform(t, got, true, StatusUpdate, "generated section is missing", want)
}

func TestTransformGitIgnoreSectionRejectsMalformedMarkersBeforeConfigDecision(t *testing.T) {
	start := "# start driftline from old/repo/.driftline/contract.toml"
	otherStart := "# start driftline from other/repo/.driftline/contract.toml"
	end := "# end driftline"
	tests := []struct {
		name    string
		current string
	}{
		{name: "missing end", current: start},
		{name: "missing start with final unterminated end", current: "entry\n" + end},
		{name: "reversed", current: end + "\n" + start + "\n"},
		{name: "nested start", current: start + "\n" + otherStart + "\n" + end + "\n"},
		{name: "duplicate complete sections", current: start + "\n" + end + "\n" + otherStart + "\n" + end + "\n"},
		{name: "recognized end inserted in owned content", current: start + "\nentry\n" + end + "\nentry\n" + end + "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := transformGitIgnoreSection([]byte(tt.current), false, "new/repo", nil)
			if err == nil {
				t.Fatal("expected malformed marker error")
			}
			if !strings.HasPrefix(err.Error(), "invalid driftline section in .gitignore:") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestTransformGitIgnoreSectionPreservesNearMissMarkersWhenAppending(t *testing.T) {
	current := []byte(" # start driftline from old/repo/.driftline/contract.toml\n" +
		"# start driftline from old/repo/.driftline/contract.toml \n" +
		"# START driftline from old/repo/.driftline/contract.toml\n" +
		" # end driftline\n" +
		"# end driftline \n" +
		"# END driftline\n")
	got, err := transformGitIgnoreSection(current, false, "new/repo", &ContractGitIgnore{Entries: []string{".env"}})
	if err != nil {
		t.Fatalf("transform .gitignore: %v", err)
	}

	want := append(append([]byte(nil), current...), []byte("\n"+generatedGitIgnoreBlockLF)...)
	requireGitIgnoreTransform(t, got, true, StatusUpdate, "generated section is missing", want)
}

func requireGitIgnoreTransform(t *testing.T, got gitIgnoreTransform, wantChanged bool, wantStatus Status, wantReason string, wantBytes []byte) {
	t.Helper()
	if got.Changed != wantChanged {
		t.Errorf("Changed = %t, want %t", got.Changed, wantChanged)
	}
	if got.Status != wantStatus {
		t.Errorf("Status = %q, want %q", got.Status, wantStatus)
	}
	if got.Reason != wantReason {
		t.Errorf("Reason = %q, want %q", got.Reason, wantReason)
	}
	if !bytes.Equal(got.DesiredBytes, wantBytes) {
		t.Errorf("DesiredBytes mismatch\ngot:  %q\nwant: %q", got.DesiredBytes, wantBytes)
	}
}
