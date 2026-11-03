package tools

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"swcode/daemon/hosts"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
)

const (
	defaultTimeoutMs = 120_000
	minTimeoutMs     = 1000
	maxTimeoutMs     = 600_000
	msPerSecond      = 1000
	noOutput         = "(no output)"
	noJobs           = "Background commands run in the main agent only."
	// startWait: a job's first seconds show whether it started well, a server says where it listens, a mistake
	// fails at once.
	startWait    = 3 * time.Second
	startPoll    = 250 * time.Millisecond
	nothingNew   = "(nothing new)"
	maxMatches   = 200
	maxFiles     = 20_000
	maxFileBytes = 1_048_576
	maxLineChars = 300
	allFiles     = "**/*"
	ignoreCase   = "i"
	maxGlobFiles = 300
)

type bashInput struct {
	Background  bool     `json:"background"`
	Command     string   `json:"command"`
	Description string   `json:"description"`
	TimeoutMs   *float64 `json:"timeoutMs"`
}

func endingOf(result hosts.CommandResult, timeoutMs int) string {
	switch {
	case result.TimedOut:
		return fmt.Sprintf("[stopped after %d s]", int(math.Round(float64(timeoutMs)/msPerSecond)))
	case result.Stopped:
		return "[stopped]"
	case result.ExitCode != nil && *result.ExitCode == 0:
		return ""
	}
	return "[exit code " + exitText(result.ExitCode) + "]"
}

func exitText(code *int) string {
	if code == nil {
		return "unknown"
	}
	return fmt.Sprint(*code)
}

func commandMeta(result hosts.CommandResult) string {
	switch {
	case result.TimedOut:
		return "timed out"
	case result.Stopped:
		return "stopped"
	case result.ExitCode != nil && *result.ExitCode == 0:
		return "done"
	}
	return "exit code " + exitText(result.ExitCode)
}

func joined(parts ...string) string {
	kept := []string{}
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "\n")
}

func commandOutcome(result hosts.CommandResult, timeoutMs int) Outcome {
	text := joined(strings.TrimRight(result.Output, " \t\r\n"), endingOf(result, timeoutMs))
	if text == "" {
		text = noOutput
	}
	succeeded := result.ExitCode != nil && *result.ExitCode == 0 && !result.TimedOut && !result.Stopped
	return Outcome{Display: Text(text), Meta: commandMeta(result), OK: succeeded, Output: text}
}

var bashTool = Define(Spec[bashInput]{
	Check: func(_ context.Context, input bashInput, tc *Context) Check {
		var preview *protocol.ToolDisplay
		if input.Description != "" {
			preview = Text(input.Description)
		}
		if input.Background {
			if tc.Jobs == nil {
				return Refuse(noJobs)
			}
			return Ready(func(ctx context.Context) Outcome { return startJob(ctx, tc.Jobs, input.Command) }, preview)
		}
		timeoutMs := whole(input.TimeoutMs, defaultTimeoutMs)
		return Ready(func(ctx context.Context) Outcome {
			options := hosts.CommandOptions{Cwd: tc.Workspace, Timeout: time.Duration(timeoutMs) * time.Millisecond}
			return commandOutcome(tc.Host.Run(ctx, input.Command, options), timeoutMs)
		}, preview)
	},
	Description: "Runs a shell command in the workspace: tests, builds, git. It gets no input, so use non-interactive flags. Give a short description. A server or a watcher runs with background, never with &: read what it prints with output, stop it with kill.",
	Input: Object(
		Optional("background", Boolean().Describe("For a command that does not end by itself (a dev server, a watcher): it runs on in the background, and the call returns after a few seconds with its first output and a job id.")),
		Required("command", String().Min(1).Describe("The command, run with bash in the workspace, without any input.")),
		Optional("description", String().Describe("What the command does, in a few words, for the person who approves it.")),
		Optional("timeoutMs", Number().Between(minTimeoutMs, maxTimeoutMs).Describe("Stops the command after this many milliseconds; 120000 when left out.")),
	),
	Name: protocol.ToolNameBash,
	Subject: func(input bashInput, _ *Context) permissions.Subject {
		return permissions.Subject{Access: permissions.AccessCommand, Command: input.Command}
	},
	Summarize: func(input bashInput) string { return input.Command },
})

func jobEnding(state *hosts.JobState) string {
	if state.ExitCode == nil {
		return "ended"
	}
	return fmt.Sprintf("exit code %d", *state.ExitCode)
}

func jobStanding(id string, state *hosts.JobState) string {
	if state.Running {
		return fmt.Sprintf("[job %s still running: read on with output, stop it with kill]", id)
	}
	return fmt.Sprintf("[job %s %s]", id, jobEnding(state))
}

// jobOutcome is what a job printed since it was last read, and how it stands.
func jobOutcome(id string, state *hosts.JobState) Outcome {
	dropped := ""
	if state.Dropped > 0 {
		dropped = fmt.Sprintf("[… %d earlier characters dropped …]", state.Dropped)
	}
	output := strings.TrimRight(state.Output, " \t\r\n")
	if output == "" {
		output = nothingNew
	}
	text := joined(dropped, output, jobStanding(id, state))
	meta := jobEnding(state)
	if state.Running {
		meta = "running"
	}
	succeeded := state.Running || (state.ExitCode != nil && *state.ExitCode == 0)
	return Outcome{Display: Text(text), Meta: meta, OK: succeeded, Output: text}
}

func unknownJob(id string) Outcome {
	return Failure(fmt.Sprintf("There is no job %s in this session.", id), "unknown job")
}

// startJob starts a command in the background, and gives what it printed in its first seconds, with its id.
func startJob(ctx context.Context, jobs Jobs, command string) Outcome {
	id, err := jobs.Start(ctx, command)
	if err != nil {
		return Failure(err.Error(), "failed")
	}
	deadline := time.Now().Add(startWait)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		state, err := jobs.Read(ctx, id, false)
		if err != nil || state == nil || !state.Running {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(startPoll):
		}
	}
	state, err := jobs.Read(context.WithoutCancel(ctx), id, true)
	if err != nil || state == nil {
		return unknownJob(id)
	}
	return jobOutcome(id, state)
}

type jobInput struct {
	ID string `json:"id"`
}

var jobInputSchema = Object(Required("id", String().Min(1).Describe("The job’s id, as bash gave it.")))

func jobSummary(input jobInput) string { return "job " + input.ID }

// outputTool reads what a background command printed since it was last read.
var outputTool = Define(Spec[jobInput]{
	Check: func(_ context.Context, input jobInput, tc *Context) Check {
		if tc.Jobs == nil {
			return Refuse(noJobs)
		}
		return Ready(func(ctx context.Context) Outcome {
			state, err := tc.Jobs.Read(ctx, input.ID, true)
			if err != nil || state == nil {
				return unknownJob(input.ID)
			}
			return jobOutcome(input.ID, state)
		}, nil)
	},
	Description: "Reads what a background command printed since you last read it, and whether it still runs.",
	Input:       jobInputSchema,
	Name:        protocol.ToolNameOutput,
	Subject:     reads[jobInput],
	Summarize:   jobSummary,
})

// killTool stops a background command and everything it started.
var killTool = Define(Spec[jobInput]{
	Check: func(_ context.Context, input jobInput, tc *Context) Check {
		if tc.Jobs == nil {
			return Refuse(noJobs)
		}
		return Ready(func(ctx context.Context) Outcome {
			state, err := tc.Jobs.Stop(ctx, input.ID)
			if err != nil || state == nil {
				return unknownJob(input.ID)
			}
			stopped := *state
			stopped.Running = false
			outcome := jobOutcome(input.ID, &stopped)
			outcome.Meta, outcome.OK = "stopped", true
			return outcome
		}, nil)
	},
	Description: "Stops a background command and everything it started, once you are done with it.",
	Input:       jobInputSchema,
	Name:        protocol.ToolNameKill,
	Subject:     reads[jobInput],
	Summarize:   jobSummary,
})

type grepInput struct {
	IgnoreCase bool   `json:"ignoreCase"`
	Include    string `json:"include"`
	Path       string `json:"path"`
	Pattern    string `json:"pattern"`
}

// inlineFlags: models write ripgrep's inline flags, (?i) first of all; JavaScript takes them as flags beside the
// pattern.
var inlineFlags = regexp.MustCompile(`^\(\?([ims]+)\)`)

// PatternOf is a regular expression from the model's pattern: its leading inline flags, if any, become flags.
func PatternOf(pattern string, caseless bool) (string, string, bool) {
	flags := map[string]bool{}
	if found := inlineFlags.FindStringSubmatch(pattern); found != nil {
		for _, flag := range found[1] {
			flags[string(flag)] = true
		}
	}
	if caseless {
		flags[ignoreCase] = true
	}
	letters := []string{}
	for flag := range flags {
		letters = append(letters, flag)
	}
	sort.Strings(letters)
	source := inlineFlags.ReplaceAllString(pattern, "")
	if _, err := hosts.CompilePattern(source, strings.Join(letters, "")); err != nil {
		return "", "", false
	}
	return source, strings.Join(letters, ""), true
}

func searchOutcome(matches []string) Outcome {
	capped := ""
	if len(matches) >= maxMatches {
		capped = fmt.Sprintf("\n[stopped at %d matches: narrow the search]", maxMatches)
	}
	output := "No matches."
	if len(matches) > 0 {
		output = strings.Join(matches, "\n") + capped
	}
	return Outcome{Display: Text(output), Meta: counted(len(matches), "match", "matches"), OK: true, Output: output}
}

func orHere(path string) string {
	if path == "" {
		return hereName
	}
	return path
}

var grepTool = Define(Spec[grepInput]{
	Check: func(ctx context.Context, input grepInput, tc *Context) Check {
		source, flags, valid := PatternOf(input.Pattern, input.IgnoreCase)
		if !valid {
			return Refuse(input.Pattern + " is not a valid regular expression.")
		}
		root, problem := resolveTarget(ctx, tc, orHere(input.Path))
		if problem != "" {
			return Refuse(problem)
		}
		if info, err := tc.Host.FileInfo(ctx, root); err != nil || info.Kind == hosts.KindMissing {
			return Refuse(input.Path + " does not exist.")
		}
		include := input.Include
		if include == "" {
			include = allFiles
		}
		return Ready(func(ctx context.Context) Outcome {
			found, err := tc.Host.Search(ctx, hosts.SearchRequest{
				Flags: flags, Include: include, MaxFileBytes: maxFileBytes, MaxFiles: maxFiles,
				MaxMatches: maxMatches, Root: root, Source: source,
			})
			if err != nil {
				return Failure(err.Error(), "failed")
			}
			lines := []string{}
			for _, match := range found {
				lines = append(lines, fmt.Sprintf("%s:%d: %s", WorkspacePath(tc.Host.Paths(), tc.Workspace, match.File), match.Line, clipped(strings.TrimSpace(match.Text), maxLineChars)))
			}
			return searchOutcome(lines)
		}, nil)
	},
	Description: "Searches file contents with a regular expression and lists the matching lines as path:line: text. Skips dependency and build folders.",
	Input: Object(
		Optional("ignoreCase", Boolean().Describe("Match without regard to case.")),
		Optional("include", String().Describe("Only search the files matching this glob, such as **/*.ts.")),
		Optional("path", searchRoot),
		Required("pattern", String().Min(1).Describe("A regular expression, in JavaScript syntax; a leading (?i) makes it ignore case.")),
	),
	Name:    protocol.ToolNameGrep,
	Subject: reads[grepInput],
	Summarize: func(input grepInput) string {
		summary := `"` + input.Pattern + `"`
		if input.Path != "" {
			summary += " in " + input.Path
		}
		if input.Include != "" {
			summary += ", " + input.Include
		}
		return summary
	},
})

type globInput struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern"`
}

func globOutcome(paths []string) Outcome {
	shown := paths[:min(len(paths), maxGlobFiles)]
	capped := ""
	if len(paths) > maxGlobFiles {
		capped = fmt.Sprintf("\n[more than %d files: narrow the pattern]", maxGlobFiles)
	}
	output := "No files match."
	if len(shown) > 0 {
		output = strings.Join(shown, "\n") + capped
	}
	return Outcome{Display: Text(output), Meta: counted(len(shown), "file", "files"), OK: true, Output: output}
}

var globTool = Define(Spec[globInput]{
	Check: func(ctx context.Context, input globInput, tc *Context) Check {
		root, problem := resolveTarget(ctx, tc, orHere(input.Path))
		if problem != "" {
			return Refuse(problem)
		}
		if info, err := tc.Host.FileInfo(ctx, root); err != nil || info.Kind != hosts.KindDirectory {
			return Refuse(input.Path + " is not a folder.")
		}
		return Ready(func(ctx context.Context) Outcome {
			files, err := tc.Host.ListFiles(ctx, root, input.Pattern, maxGlobFiles+1)
			if err != nil {
				return Failure(err.Error(), "failed")
			}
			paths := []string{}
			for _, file := range files {
				paths = append(paths, WorkspacePath(tc.Host.Paths(), tc.Workspace, file))
			}
			sort.Strings(paths)
			return globOutcome(paths)
		}, nil)
	},
	Description: "Lists the files matching a glob pattern, from the workspace or a folder in it. Skips dependency and build folders.",
	Input: Object(
		Optional("path", searchRoot),
		Required("pattern", String().Min(1).Describe("A glob such as src/**/*.ts or *.json.")),
	),
	Name:    protocol.ToolNameGlob,
	Subject: reads[globInput],
	Summarize: func(input globInput) string {
		if input.Path != "" {
			return input.Pattern + " in " + input.Path
		}
		return input.Pattern
	},
})
