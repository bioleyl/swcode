package skills

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"swcode/daemon/protocol"
)

const (
	// MaxFiles: a skill's folder may hold many files, and the agent is told of the first ones.
	MaxFiles = 50
	// maxFileChars is as much of a file as a web page gives the reader: enough for a reference, not a whole data set.
	maxFileChars = 60_000
	baseFolder   = "Base directory for this skill:"
)

// Text is a skill, or a file of its folder, as the agent reads it; or why it cannot be read.
type Text struct {
	OK      bool
	Text    string
	Problem string
}

// Clipped is a file of a skill's folder, cut where a reference would end.
func Clipped(text string) string {
	length := utf8.RuneCountInString(text)
	if length <= maxFileChars {
		return text
	}
	return fmt.Sprintf("%s\n[The file goes on for %d more characters.]", string([]rune(text)[:maxFileChars]), length-maxFileChars)
}

func UnknownSkill(name string, offered []protocol.SkillSummary) string {
	if len(offered) == 0 {
		return fmt.Sprintf("There is no skill named %s: the user has none.", name)
	}
	names := make([]string, len(offered))
	for index, skill := range offered {
		names[index] = skill.Name
	}
	return fmt.Sprintf("There is no skill named %s. The skills: %s.", name, strings.Join(names, ", "))
}

func filesNote(files []string) string {
	if len(files) == 0 {
		return ""
	}
	return "\n\n[Files of this skill, to read with skill and file: " + strings.Join(files, ", ") + "]"
}

// LoadedText is a skill as the agent reads it: where its folder is (its scripts are run from it), its
// instructions, then the files of its folder.
func LoadedText(folder, instructions string, files []string) string {
	return baseFolder + " " + folder + "\n\n" + instructions + filesNote(files)
}
