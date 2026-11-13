package tools

import (
	"slices"

	"swcode/daemon/llm"
	"swcode/daemon/protocol"
)

// all is every tool, by name: a new tool name does not get offered until it is here.
var all = map[protocol.ToolName]Tool{
	protocol.ToolNameAgent: agentTool, protocol.ToolNameBash: bashTool, protocol.ToolNameDraw: drawTool, protocol.ToolNameEdit: editTool,
	protocol.ToolNameGlob: globTool, protocol.ToolNameGrep: grepTool, protocol.ToolNameImage: imageTool,
	protocol.ToolNameKill: killTool, protocol.ToolNameNotes: notesTool, protocol.ToolNameOutput: outputTool, protocol.ToolNamePlan: planTool,
	protocol.ToolNameQuestion: questionTool, protocol.ToolNameRead: readTool,
	protocol.ToolNameScreenshot: screenshotTool, protocol.ToolNameSkill: skillTool, protocol.ToolNameTerminal: terminalTool,
	protocol.ToolNameReadterminal: terminalOutputTool, protocol.ToolNameTodo: todoTool, protocol.ToolNameClosetodo: closeTodoTool,
	protocol.ToolNameWebfetch: webfetchTool, protocol.ToolNameWebsearch: websearchTool, protocol.ToolNameWrite: writeTool,
}

var (
	// exploreTools: an explore subagent looks without touching, nothing it does needs a person's approval.
	exploreTools = []protocol.ToolName{
		protocol.ToolNameRead, protocol.ToolNameGrep, protocol.ToolNameGlob, protocol.ToolNameWebsearch, protocol.ToolNameWebfetch,
	}
	// chatTools: a chat reads the web and draws, nothing else.
	chatTools = []protocol.ToolName{protocol.ToolNameWebsearch, protocol.ToolNameWebfetch, protocol.ToolNameDraw}
	// chatOnlyTools are a chat's alone: the agent saves its pictures in the workspace with image.
	chatOnlyTools = []protocol.ToolName{protocol.ToolNameDraw}
	// imageTools need a picture server of the settings.
	imageTools = []protocol.ToolName{protocol.ToolNameImage, protocol.ToolNameDraw}
	// browserTools need a browser.
	browserTools = []protocol.ToolName{protocol.ToolNameScreenshot}
	// retiredTools are offered no longer: their names stay for the calls older sessions hold.
	retiredTools = []protocol.ToolName{protocol.ToolNameSaveskill}
	// serverTools are no tool of swcode's own: `mcp` names every tool of an MCP server, which a box is given beside
	// its own (With).
	serverTools = []protocol.ToolName{protocol.ToolNameMcp}
	// waitingTools only read, and mostly wait while they do (a disk, a host over SSH, a page): the calls of one
	// reply to them run together. They ask nobody anything and change nothing, so no order between them matters.
	waitingTools = []protocol.ToolName{
		protocol.ToolNameRead, protocol.ToolNameGrep, protocol.ToolNameGlob, protocol.ToolNameWebsearch, protocol.ToolNameWebfetch,
	}
)

// Box is the tools an agent has: as the model reads them, and by name. Its own tools, then the ones of the MCP
// servers it was given.
type Box struct {
	names []protocol.ToolName
	more  []Tool
}

// tools is the box's tools, its own first.
func (b Box) tools() []Tool {
	tools := make([]Tool, 0, len(b.names)+len(b.more))
	for _, name := range b.names {
		tools = append(tools, all[name])
	}
	return append(tools, b.more...)
}

func (b Box) Definitions() []llm.ToolDefinition {
	definitions := []llm.ToolDefinition{}
	for _, tool := range b.tools() {
		definitions = append(definitions, llm.ToolDefinition{Description: tool.Description, Name: tool.functionName(), Parameters: tool.parameters()})
	}
	return definitions
}

// Without is the box but one tool: the main agent's without agent, for a preset that allows no subagents.
func (b Box) Without(name protocol.ToolName) Box {
	return Box{more: b.more, names: slices.DeleteFunc(slices.Clone(b.names), func(each protocol.ToolName) bool { return each == name })}
}

// With is the box with the tools of MCP servers beside its own.
func (b Box) With(more []Tool) Box {
	return Box{more: slices.Concat(b.more, more), names: b.names}
}

// Find is the tool the model calls by that name, when the agent has it.
func (b Box) Find(name string) (Tool, bool) {
	for _, tool := range b.tools() {
		if tool.functionName() == name {
			return tool, true
		}
	}
	return Tool{}, false
}

// RunsTogether tells whether calls to that tool may run at the same time as other calls of the same reply: it only
// reads, and the agent has it.
func (b Box) RunsTogether(name string) bool {
	_, has := b.Find(name)
	return has && slices.Contains(waitingTools, protocol.ToolName(name))
}

// Servers is which servers of the settings the agents have.
type Servers struct {
	Images  bool
	Browser bool
}

// Boxes is the main agent's, the subagents' and a chat's tools, without those whose server is not set up.
func Boxes(servers Servers) (Box, Box, Box) {
	missing := []protocol.ToolName{}
	if !servers.Images {
		missing = append(missing, imageTools...)
	}
	if !servers.Browser {
		missing = append(missing, browserTools...)
	}
	available := func(names []protocol.ToolName) Box {
		kept := []protocol.ToolName{}
		for _, name := range names {
			if !slices.Contains(missing, name) {
				kept = append(kept, name)
			}
		}
		return Box{names: kept}
	}
	return available(mainTools), available(exploreTools), available(chatTools)
}

// mainTools is every tool but a chat's own and those retired.
var mainTools = slices.DeleteFunc(slices.Clone(protocol.ToolNames), func(name protocol.ToolName) bool {
	return slices.Contains(chatOnlyTools, name) || slices.Contains(retiredTools, name) || slices.Contains(serverTools, name)
})

// Main is the main agent's tools, with all servers.
var Main = Box{names: mainTools}

// Explore is an explore subagent's tools: reading and searching only.
var Explore = Box{names: exploreTools}
