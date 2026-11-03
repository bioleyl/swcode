package skills

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"swcode/daemon/protocol"
)

const (
	// maxFileChars is as much of a file as a web page gives the reader: enough for a reference, not a whole data set.
	maxFileChars = 60_000
	baseFolder   = "Base directory for this skill:"
)

// Clipped is a file of a skill's folder, cut where a reference would end.
func Clipped(text string) string {
	length := utf8.RuneCountInString(text)
	if length <= maxFileChars {
		return text
	}
	return fmt.Sprintf("%s\n[The file goes on for %d more characters.]", string([]rune(text)[:maxFileChars]), length-maxFileChars)
}

func NoSkill(name string) string {
	return fmt.Sprintf("There is no skill named %s.", name)
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

func ForPeopleOnly(name string) string {
	return name + " is for the person to type as a command in Claude Code: its author keeps it from agents."
}

func filesNote(files []string) string {
	if len(files) == 0 {
		return ""
	}
	return "\n\n[Files of this skill, to read with skill and file: " + strings.Join(files, ", ") + "]"
}

// LoadedText is a skill as the agent reads it: where its folder is when commands run there (its scripts are run
// from it, as Claude Code says), its instructions, then the files of its folder.
func LoadedText(folder, instructions string, files []string) string {
	base := ""
	if folder != "" {
		base = baseFolder + " " + folder + "\n\n"
	}
	return base + instructions + filesNote(files)
}
