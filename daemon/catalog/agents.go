package catalog

import (
	"context"
	"regexp"
	"strings"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/skills"
)

const (
	// ownAgentsFolder is where the person's own agents are, beside the skills: .agents/agents, a file each.
	ownAgentsFolder = "agents"
	agentFiles      = "*.md"
	agentSuffix     = ".md"
	maxAgents       = 100
)

var (
	agentFrontmatter = regexp.MustCompile(`^---\r?\n([\s\S]*?)\r?\n---[ \t]*(?:\r?\n|$)`)
	// toolsKey is the frontmatter's list of tools: on its line (read, write or [read, write]) or as items under it.
	toolsKey   = regexp.MustCompile(`(?m)^tools:[ \t]*(.*)$((?:\r?\n[ \t]+-[^\n]*)*)`)
	toolsSplit = regexp.MustCompile(`[\s,\[\]"'-]+`)
)

// HostAgent is an agent the person defined, as its file on the project's host says: what it is for, the
// instructions it starts from, and the tools it works with (none named: an explore subagent's).
type HostAgent struct {
	Name         string
	Description  string
	Instructions string
	Tools        []string
	Origin       protocol.SkillOrigin
}

// toolsOf is the tools an agent's frontmatter names, as written.
func toolsOf(text string) []string {
	block := agentFrontmatter.FindStringSubmatch(text)
	if block == nil {
		return nil
	}
	found := toolsKey.FindStringSubmatch(block[1])
	if found == nil {
		return nil
	}
	names := []string{}
	for _, name := range toolsSplit.Split(found[1]+" "+found[2], -1) {
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// agentsUnder is the agents of a folder's .agents/agents: its .md files, named by their frontmatter or their file.
func (s shelf) agentsUnder(folder string) []HostAgent {
	paths := s.host.Paths()
	root := paths.Join(folder, agentsFolder, ownAgentsFolder)
	files, err := s.host.ListFiles(s.ctx, root, agentFiles, maxAgents)
	if err != nil {
		return nil
	}
	found := []HostAgent{}
	for _, file := range files {
		// An agent is a file of the folder itself: a folder under it is not looked into.
		if paths.Dir(file) != root {
			continue
		}
		text, ok := s.readText(file)
		if !ok {
			continue
		}
		parsed := skills.Parse(text)
		name := strings.TrimSpace(parsed.Name)
		if name == "" {
			name = strings.TrimSuffix(paths.Base(file), agentSuffix)
		}
		found = append(found, HostAgent{
			Description: parsed.Description, Instructions: parsed.Instructions, Name: name, Origin: s.origin, Tools: toolsOf(text),
		})
	}
	return found
}

// FindAgents is the agents the person defined, on a host: theirs in ~/.agents/agents, then the project's in the
// .agents/agents of its folder and of its parents up to the repository's root, each name once, the person's first.
// personalOnly leaves the project's own out: those of a folder the person has not trusted.
func FindAgents(ctx context.Context, host hosts.Host, workspace string, personalOnly bool) []HostAgent {
	home, err := host.Home(ctx)
	if err != nil {
		return nil
	}
	all := shelf{ctx: ctx, host: host, origin: protocol.SkillOriginPersonal}.agentsUnder(home)
	if workspace != "" && !personalOnly {
		project := shelf{ctx: ctx, host: host, origin: protocol.SkillOriginProject}
		for _, folder := range ProjectFolders(ctx, host, workspace) {
			all = append(all, project.agentsUnder(folder)...)
		}
	}
	kept := []HostAgent{}
	seen := map[string]bool{}
	for _, agent := range all {
		if !seen[agent.Name] {
			seen[agent.Name] = true
			kept = append(kept, agent)
		}
	}
	return kept
}
