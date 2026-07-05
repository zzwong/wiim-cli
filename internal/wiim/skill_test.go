package wiim

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/spf13/cobra"
)

// collectCommandNames walks the cobra command tree and records the leaf name
// (cobra's Name(), the first whitespace-separated token of Use) of every
// command and command group.
func collectCommandNames(cmd *cobra.Command, names map[string]bool) {
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
		collectCommandNames(c, names)
	}
}

// TestSkillDocMentionsAllCommands guards against skills/wiim/SKILL.md drifting
// from the actual command tree: a command added to cli.go with no mention in
// SKILL.md is invisible to an agent relying on the skill for safety guidance
// (see the missing seek/clear/prompt-url/play-file/spotify-play/spotify-transfer
// entries fixed alongside this test).
func TestSkillDocMentionsAllCommands(t *testing.T) {
	a := newApp(io.Discard, io.Discard)
	names := map[string]bool{}
	collectCommandNames(a.root, names)

	skillPath := filepath.Join("..", "..", "skills", "wiim", "SKILL.md")
	content, err := os.ReadFile(skillPath) // #nosec G304 -- fixed repo-relative path, not user input
	if err != nil {
		t.Fatalf("reading %s: %v", skillPath, err)
	}
	text := string(content)

	var missing []string
	for name := range names {
		if !mentionsCommandName(text, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("skills/wiim/SKILL.md does not mention these commands: %v\n"+
			"Add them so an agent relying on the skill knows they exist, and — if they "+
			"mutate device or account state — that they need explicit permission first.",
			missing)
	}
}

// mentionsCommandName reports whether name appears in text as a standalone
// token. A plain \bname\b regex isn't enough: regexp word boundaries treat
// "-" as a delimiter, so \bplay\b matches inside "play-url"/"play-m3u"/
// "play-file" even though those are distinct commands from "play". Treating
// "-" (and "_") as part of the "word" for boundary purposes avoids a
// hyphenated sibling command silently satisfying the check for a shorter
// command name that's actually undocumented.
func mentionsCommandName(text, name string) bool {
	pattern := `(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(name) + `($|[^A-Za-z0-9_-])`
	return regexp.MustCompile(pattern).MatchString(text)
}
