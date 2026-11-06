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
	protocol.ToolNameKill: killTool, protocol.ToolNameOutput: outputTool, protocol.ToolNamePlan: planTool,
	protocol.ToolNameQuestion: questionTool, protocol.ToolNameRead: readTool, protocol.ToolNameSaveskill: saveSkillTool,
	protocol.ToolNameScreenshot: screenshotTool, protocol.ToolNameSkill: skillTool,
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
)

// Box is the tools an agent has: as the model reads them, and by name.
type Box struct {
	names []protocol.ToolName
}

func (b Box) Definitions() []llm.ToolDefinition {
	definitions := []llm.ToolDefinition{}
	for _, name := range b.names {
		tool := all[name]
		definitions = append(definitions, llm.ToolDefinition{Description: tool.Description, Name: string(name), Parameters: tool.Input.Parameters()})
	}
	return definitions
}

// Without is the box but one tool: the main agent's without agent, for a preset that allows no subagents.
func (b Box) Without(name protocol.ToolName) Box {
	return Box{names: slices.DeleteFunc(slices.Clone(b.names), func(each protocol.ToolName) bool { return each == name })}
}

// Find is the tool of that name, when the agent has it.
func (b Box) Find(name string) (Tool, bool) {
	if !slices.Contains(b.names, protocol.ToolName(name)) {
		return Tool{}, false
	}
	return all[protocol.ToolName(name)], true
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

// mainTools is every tool but a chat's own.
var mainTools = slices.DeleteFunc(slices.Clone(protocol.ToolNames), func(name protocol.ToolName) bool {
	return slices.Contains(chatOnlyTools, name)
})

// Main is the main agent's tools, with all servers.
var Main = Box{names: mainTools}

// Explore is an explore subagent's tools: reading and searching only.
var Explore = Box{names: exploreTools}
