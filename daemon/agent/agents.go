package agent

import (
	"strings"

	"swcode/daemon/hosts"
	"swcode/daemon/permissions"
)

// DefinedAgent is an agent the person defined, as a subagent runs it: the instructions it starts from, and its
// session's permissions and sandbox, under which it works with no one to ask.
type DefinedAgent struct {
	Name         string
	Instructions string
	// Grants is what the session lets through without asking, read at each call: its mode, what people allowed
	// for it, and its project's rules.
	Grants func() permissions.Grants
	// Sandbox is where the session's commands run; nil where they run as the person does.
	Sandbox *hosts.Sandbox
	// Scratch is the session's scratch folder, which its sandbox may write.
	Scratch string
}

// DefinedAgentPrompt is what an agent the person defined is told: their instructions first, then where it works
// and how a subagent reports.
func DefinedAgentPrompt(instructions, workspace, platform string) string {
	return strings.Join(nonEmpty(
		strings.TrimSpace(instructions),
		strings.Join([]string{
			"You are a subagent of swcode, a coding agent. Another agent gave you a task: do it with the tools you have, and report.",
			"",
			"Workspace: " + workspace + " (relative paths start here)",
			"Platform: " + platform,
			"",
			"How to work:",
			"- Read a file with read before you edit it, and search with grep and glob instead of guessing paths.",
			"- Nobody can approve a call for you: one the session does not let through ends your task, so stay within what the task needs.",
			webDataLine,
			"- Your last message is all the other agent gets: say what you did or found, with file paths, in a few lines.",
		}, "\n"),
	), "\n\n")
}
