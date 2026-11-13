package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"swcode/daemon/protocol"
)

const (
	skillsHeader  = "The user’s skills: their instructions for kinds of tasks. When one fits the task, load it with skill before you start, and follow it."
	noSkills      = "The user has no skills now."
	noDescription = "no description"
	agentsHeader  = "The user’s agents: subagents they defined, each with its own instructions and tools. Give one a task with agent, naming it in agent."
	loadedHeader  = "The user ran /%s: that skill is loaded below, for their message. Follow it."
)

// skillCommand is a message that runs a skill: a slash and its name, first, then what the person wrote, if anything.
var skillCommand = regexp.MustCompile(`^/([A-Za-z0-9][\w.:-]*)(?:\s|$)`)

// SkillsText is what the model reads of the skills: a line each, its name and what it is for.
func SkillsText(skills []protocol.SkillSummary) string {
	if len(skills) == 0 {
		return noSkills
	}
	lines := []string{skillsHeader}
	for _, skill := range skills {
		description := skill.Description
		if description == "" {
			description = noDescription
		}
		lines = append(lines, "- "+skill.Name+": "+description)
	}
	return strings.Join(lines, "\n")
}

// AgentsText is what the model reads of the agents the user defined: a line each; nothing when there are none.
func AgentsText(agents []protocol.SkillSummary) string {
	if len(agents) == 0 {
		return ""
	}
	lines := []string{agentsHeader}
	for _, each := range agents {
		description := each.Description
		if description == "" {
			description = noDescription
		}
		lines = append(lines, "- "+each.Name+": "+description)
	}
	return strings.Join(lines, "\n")
}

// ListedText is what the model reads when it is told the skills: them, then the user's agents when there are some.
func ListedText(listed protocol.SessionEventSkillsListed) string {
	return strings.Join(nonEmpty(SkillsText(listed.Skills), AgentsText(listed.Agents)), "\n\n")
}

func sameSkills(known, skills []protocol.SkillSummary) bool {
	if len(known) != len(skills) {
		return false
	}
	for index := range known {
		if known[index] != skills[index] {
			return false
		}
	}
	return true
}

func lastListed(events []protocol.SessionEvent) protocol.SessionEventSkillsListed {
	recent := SinceCompaction(events)
	for index := len(recent) - 1; index >= 0; index-- {
		if listed, ok := recent[index].Value.(protocol.SessionEventSkillsListed); ok {
			return listed
		}
	}
	return protocol.SessionEventSkillsListed{}
}

// ListSkills tells the model the user's skills when they are not the ones it last saw, since the last compaction:
// a message at the end of the conversation, so the system prompt and what came before stay as llm-server cached
// them.
func ListSkills(ctx context.Context, host TurnHost, setup Setup) {
	if setup.Skills == nil {
		return
	}
	skills := setup.Skills(ctx)
	var agents []protocol.SkillSummary
	if setup.Agents != nil {
		agents = setup.Agents(ctx)
	}
	known := lastListed(host.History())
	if sameSkills(known.Skills, skills) && sameSkills(known.Agents, agents) {
		return
	}
	host.Record(protocol.SessionEventSkillsListed{Agents: agents, ID: setup.NewID(), Skills: append([]protocol.SkillSummary{}, skills...)})
}

// LoadedSkillText is a skill the person ran, as the model reads it after their message.
func LoadedSkillText(loaded protocol.SessionEventSkillLoaded) string {
	return fmt.Sprintf(loadedHeader, loaded.Name) + "\n\n" + loaded.Text
}

// LoadAskedSkill loads the skill a message runs with /<its name>, right after it: the model reads its instructions
// with the message, without a step to load it. A message that names none of the session's skills is a message.
func LoadAskedSkill(ctx context.Context, host TurnHost, setup Setup, text string) {
	found := skillCommand.FindStringSubmatch(strings.TrimSpace(text))
	if setup.LoadSkill == nil || found == nil {
		return
	}
	loaded := setup.LoadSkill(ctx, found[1])
	if !loaded.OK {
		return
	}
	host.Record(protocol.SessionEventSkillLoaded{ID: setup.NewID(), Name: found[1], Text: loaded.Text})
}
