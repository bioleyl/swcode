// Package tools is what the agents do: read, write and edit files, run commands, search, plan, ask, reach the web,
// make pictures, load skills, look at pages and start subagents. Each tool checks its call before anyone is asked
// about it, and goes through the session's host.
package tools

import (
	"context"
	"encoding/json"
	"sync"

	"swcode/daemon/catalog"
	"swcode/daemon/hosts"
	"swcode/daemon/images"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
	"swcode/daemon/terminals"
	"swcode/daemon/web"
)

const rawSummaryLength = 80

// ReadFiles is when each file was last read or written, by absolute path: a file changes only as the agent last
// saw it.
type ReadFiles struct {
	mu    sync.Mutex
	times map[string]int64
}

func NewReadFiles() *ReadFiles { return &ReadFiles{times: map[string]int64{}} }

func (r *ReadFiles) Get(path string) (int64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	time, ok := r.times[path]
	return time, ok
}

func (r *ReadFiles) Set(path string, time int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.times[path] = time
}

// SubagentTask is what the main agent hands a subagent: a few words for people, the whole task for the subagent.
type SubagentTask struct {
	CallID      string
	Description string
	Prompt      string
}

// Subagents starts subagents for the main agent, in the background: the call's result says one started, its report
// comes to the main agent later. A subagent itself gets none, so subagents never nest.
type Subagents interface {
	Start(task SubagentTask) Outcome
}

// ProjectTodos are the todos of the project a session works in: the agent opens one, and closes one.
type ProjectTodos interface {
	// OpenTodo adds a todo to the project of a folder on a host, and answers its todos; or why not, with them.
	OpenTodo(folder string, host protocol.HostRef, text string) ([]protocol.ProjectTodo, string)
	// CloseTodo marks done the todo that says text, and answers the project's todos; or why not, with them.
	CloseTodo(folder string, host protocol.HostRef, text string) ([]protocol.ProjectTodo, string)
}

// Jobs is a session's background commands, on its host, in its workspace.
type Jobs interface {
	Start(ctx context.Context, command string) (string, error)
	// Read is a job's state; consume takes the output read, so the next read gives what came after.
	Read(ctx context.Context, id string, consume bool) (*hosts.JobState, error)
	// Stop stops it and everything it started.
	Stop(ctx context.Context, id string) (*hosts.JobState, error)
}

// Context is what a tool call works with: where the project is, what the agent read, and the session's ports.
type Context struct {
	Host hosts.Host
	// Workspace is the folder the agent works in: relative paths start there.
	Workspace string
	// Scratch is the session's own folder outside the project, for the agent's experiments; empty for none.
	Scratch   string
	ReadFiles *ReadFiles
	// AskQuestions asks the person the agent's questions and waits for the answers; nil for a subagent.
	AskQuestions func(ctx context.Context, callID string, questions []protocol.Question) []string
	// AskPassword asks the person, under a call, the password one of its commands asked for, and waits: the words
	// typed, and whether any were. Nil where there is no one to ask, which is how a subagent's commands run.
	AskPassword func(ctx context.Context, callID, prompt string) (string, bool)
	// ShowOutput shows the person what a call's command prints while it runs; nil where nobody watches (a
	// subagent).
	ShowOutput func(callID, text string)
	// CallID is the call being prepared: a subagent's work shows under it.
	CallID    string
	Subagents Subagents
	// Web is the web search and pages, from the daemon; nil where the web cannot be reached.
	Web *web.Port
	// Images is pictures from the picture server, when there is one; subagents get none.
	Images images.Port
	// HostRef is where the session's project is: an agent's terminal runs there, in its folder.
	HostRef protocol.HostRef
	// Jobs is the session's background commands; subagents get none.
	Jobs Jobs
	// Screenshots is pages drawn by a browser, when the daemon has one; subagents get none.
	Screenshots Screenshots
	// Terminals are the shells of the app, where the agent runs what people watch; subagents get none.
	Terminals terminals.Port
	// Skills is the session's skills; subagents get none.
	Skills catalog.Port
	// Todos are the project's todos, which the agent opens and closes; nil in a chat, which works in no project.
	Todos ProjectTodos
}

// Outcome is how a call ended.
type Outcome struct {
	OK bool
	// Output is what the model reads back.
	Output string
	// Meta is the result in a few words, for people.
	Meta    string
	Display *protocol.ToolDisplay
	// Pictures are what the model reads back after the output: a screenshot it asked to see.
	Pictures []protocol.PictureRef
}

// Failure is a call that ran and failed.
func Failure(output, meta string) Outcome { return Outcome{Meta: meta, Output: output} }

// Text shows some text under a call.
func Text(text string) *protocol.ToolDisplay {
	return &protocol.ToolDisplay{Value: protocol.ToolDisplayText{Text: text}}
}

// Check is a call checked before anyone is asked about it: ready to run, with what a person sees first, or refused
// with why, for the model to correct it.
type Check struct {
	OK      bool
	Preview *protocol.ToolDisplay
	Run     func(ctx context.Context) Outcome
	Problem string
}

func Ready(run func(ctx context.Context) Outcome, preview *protocol.ToolDisplay) Check {
	return Check{OK: true, Preview: preview, Run: run}
}

func Refuse(problem string) Check { return Check{Problem: problem} }

// PreparedCall is a call ready to run once a person approves it, when one has to.
type PreparedCall struct {
	Summary string
	Subject permissions.Subject
	Preview *protocol.ToolDisplay
	Run     func(ctx context.Context) Outcome
}

// Preparation is a call prepared, or why it cannot start.
type Preparation struct {
	OK      bool
	Call    PreparedCall
	Summary string
	Problem string
}

// Tool takes the model's JSON: checked against its input schema, then handed to its spec.
type Tool struct {
	Name        protocol.ToolName
	Description string
	Input       Schema
	Prepare     func(ctx context.Context, raw any, tc *Context) Preparation
}

// Spec is a tool written against its own input type.
type Spec[Input any] struct {
	Name        protocol.ToolName
	Description string
	Input       Schema
	Summarize   func(input Input) string
	Subject     func(input Input, tc *Context) permissions.Subject
	// Check checks the call before anyone is asked about it: a call bound to fail goes back to the model at once.
	Check func(ctx context.Context, input Input, tc *Context) Check
}

func rawSummary(raw any) string {
	text, _ := json.Marshal(raw)
	runes := []rune(string(text))
	return string(runes[:min(len(runes), rawSummaryLength)])
}

// Define makes a tool of a spec.
func Define[Input any](spec Spec[Input]) Tool {
	return Tool{
		Description: spec.Description,
		Input:       spec.Input,
		Name:        spec.Name,
		Prepare: func(ctx context.Context, raw any, tc *Context) Preparation {
			checked, err := spec.Input.Check(raw)
			if err != nil {
				return Preparation{Problem: "The input does not fit: " + err.Error(), Summary: rawSummary(raw)}
			}
			var input Input
			data, _ := json.Marshal(checked)
			if err := json.Unmarshal(data, &input); err != nil {
				return Preparation{Problem: "The input does not fit: " + err.Error(), Summary: rawSummary(raw)}
			}
			summary := spec.Summarize(input)
			check := spec.Check(ctx, input, tc)
			if !check.OK {
				return Preparation{Problem: check.Problem, Summary: summary}
			}
			return Preparation{OK: true, Call: PreparedCall{
				Preview: check.Preview, Run: check.Run, Subject: spec.Subject(input, tc), Summary: summary,
			}, Summary: summary}
		},
	}
}

// reads is the subject of a tool that only reads.
func reads[Input any](Input, *Context) permissions.Subject { return permissions.Read }
