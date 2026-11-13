package sessions

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"swcode/daemon/agent"
	"swcode/daemon/catalog"
	"swcode/daemon/hosts"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
	"swcode/daemon/tools"
)

const (
	// agentsLimit is how long finding the person's agents on the session's host may take, when a call asks for one
	// the turn has not read yet.
	agentsLimit  = 20 * time.Second
	unknownAgent = "There is no agent named %s. The user’s agents: %s. Leave agent out for an explore subagent."
	noAgents     = "The user has defined no agent: leave agent out for an explore subagent."
)

// definedAgents is the agents the person defined, as a turn finds them on its host: read once, when the model is
// told of them or first gives one a task.
type definedAgents struct {
	once  sync.Once
	find  func(ctx context.Context) []catalog.HostAgent
	found []catalog.HostAgent
}

// agentsOf is a turn's agents: the person's own, and the project's unless its folder is not trusted.
func agentsOf(host hosts.Host, workspace string, untrusted bool) *definedAgents {
	return &definedAgents{find: func(ctx context.Context) []catalog.HostAgent {
		return catalog.FindAgents(ctx, host, workspace, untrusted)
	}}
}

func (d *definedAgents) all(ctx context.Context) []catalog.HostAgent {
	d.once.Do(func() { d.found = d.find(ctx) })
	return d.found
}

// summaries is the agents as the model is told of them: a name and what it is for.
func (d *definedAgents) summaries(ctx context.Context) []protocol.SkillSummary {
	listed := []protocol.SkillSummary{}
	for _, each := range d.all(ctx) {
		listed = append(listed, protocol.SkillSummary{Description: each.Description, Name: each.Name})
	}
	return listed
}

// toldAgents is the agents a turn's model is told of: none where the preset allows no subagent, since the model has
// no agent tool to start one with.
func toldAgents(where place, limit *protocol.SubagentLimit) func(ctx context.Context) []protocol.SkillSummary {
	if where.agents == nil || (limit != nil && limit.AtOnce == 0) {
		return nil
	}
	return where.agents.summaries
}

// useDefined makes a subagent's deps those of the agent the person defined under that name: its tools, its
// instructions, and the session's permissions and sandbox, under which it works with no one to ask. It gives the
// agent's name, or why there is none to give the task to.
func (a subagents) useDefined(name string, deps *agent.SubagentDeps) (string, string) {
	if a.where.agents == nil {
		return "", noAgents
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentsLimit)
	defer cancel()
	names := []string{}
	for _, each := range a.where.agents.all(ctx) {
		if each.Name != name {
			names = append(names, each.Name)
			continue
		}
		s, session := a.service, a.session
		deps.Tools = tools.AgentBox(each.Tools)
		deps.Defined = &agent.DefinedAgent{
			Grants: func() permissions.Grants {
				s.mu.Lock()
				defer s.mu.Unlock()
				return s.grantsOf(session)
			},
			Instructions: each.Instructions, Name: each.Name, Sandbox: a.where.sandbox, Scratch: a.where.scratch,
		}
		return each.Name, ""
	}
	if len(names) == 0 {
		return "", noAgents
	}
	return "", fmt.Sprintf(unknownAgent, name, strings.Join(names, ", "))
}
