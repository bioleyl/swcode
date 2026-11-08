// Package swcodebase is the skill swcode comes with, swcode-base. The program does not carry it: swcode's repository
// publishes it (plugins/swcode-base, listed in .claude-plugin/marketplace.json for Claude Code and skills.sh), and the
// skills CLI (npx skills) puts it in a person's home or a project's .agents/skills when they ask, on the host the
// project is on, and takes it out again.
package swcodebase

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/skills"
)

const (
	// Name is the skill's name, and its folder's in .agents/skills.
	Name = "swcode-base"
	// source is where the skills CLI fetches it: swcode's repository on GitHub.
	source = "bioleyl/swcode"
	// skillsCLI is the skills CLI at the version whose options these are.
	skillsCLI = "skills@1.7.6"
	// universalAgent is the skills CLI's agent whose folder is .agents/skills itself, the project's, or the home's with
	// --global: where swcode's agent reads skills. Any other agent would get a copy in its own folder too.
	universalAgent = "universal"
	// cliTimeout is how long the CLI may take: npx fetches it first, and it clones the repository.
	cliTimeout = 5 * time.Minute
	// commandNotFound is the shell's exit code for a program it cannot find: npx, where Node.js is not installed.
	commandNotFound = 127
	// errorMark is what the CLI prints before its error.
	errorMark = "■"
	// gutter is what the CLI draws before its lines: its frame, its marks and its spinner.
	gutter = " \t│┌└├◇◆●■▲◒◐◓◑"
	// agentsFolder and skillsFolder are where the agent looks for skills, in a home or in a project.
	agentsFolder = ".agents"
	skillsFolder = "skills"
	// homeUnread is why nothing is known of a home: its host did not say where it is.
	homeUnread = "The home folder could not be read: %v."
	// noProject is why a project's skills cannot be changed: the app named no project it knows.
	noProject = "There is no such project."
	// noNpx is why nothing could be installed: the host has no npx to run the skills CLI with.
	noNpx = "npx was not found on %s: swcode-base is installed with the skills CLI, which needs Node.js 22.20 or later there."
	// cliTimedOut and cliFailed are why the CLI did not do what it was asked: the action, the host, its error.
	cliTimedOut = "npx skills could not %s swcode-base on %s: it took longer than %v."
	cliFailed   = "npx skills could not %s swcode-base on %s: %s"
	// reasonSeparator joins the CLI's error line and the one that explains it.
	reasonSeparator = ": "
	// thisComputer is the local host as a problem names it, in the middle of its sentence.
	thisComputer = "this computer"
	// commandMark is where bashHostScript holds the CLI's command line: twice, for its two branches.
	commandMark = "@command@"
	// bashHostScript runs the command line on a host whose commands run in bash, and gives a Windows npx reached
	// through WSL the host's home and folder: see commandOn. cmd.exe takes a UNC folder for its own only once a drive
	// is mapped: pushd maps the share (unquoted, or cmd calls it invalid), then the folder is reached from there, so
	// its spaces are safe.
	bashHostScript = `if command -v npx 2>/dev/null | grep -q '^/mnt/' && command -v cmd.exe >/dev/null 2>&1; then
  folder=$(wslpath -w "$PWD")
  home=$(wslpath -w "$HOME")
  case "$folder" in
    '\\'*)
      rest=${folder#\\\\}
      after=${rest#*\\}
      enter="pushd \\\\${rest%%\\*}\\${after%%\\*} && cd \"${after#*\\}\""
      ;;
    *)
      enter="cd /d \"$folder\""
      ;;
  esac
  exec cmd.exe /c "$enter && set USERPROFILE=$home&& @command@"
fi
@command@`
)

// escapes are the terminal's codes in the CLI's output: its cursor's moves, its colours.
var escapes = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

// action is what the skills CLI is asked to do with swcode-base: its words on the command line, and the verb that
// says it in a problem.
type action struct {
	verb  string
	words []string
}

var (
	adding   = action{verb: "install", words: []string{"add", source, "--skill", Name}}
	removing = action{verb: "remove", words: []string{"remove", Name}}
)

// commandOf is the skills CLI's command line for an action, in the home or in the project. Its words need no quotes,
// so bash, Git Bash and PowerShell read it alike.
func commandOf(act action, inHome bool) string {
	words := append([]string{"npx", "--yes", skillsCLI}, act.words...)
	words = append(words, "--agent", universalAgent, "--yes")
	return strings.Join(append(words, scopeOf(inHome)...), " ")
}

// scopeOf is the CLI's option for the home: without it, the CLI works in the folder it runs in, the project.
func scopeOf(inHome bool) []string {
	if !inHome {
		return nil
	}
	return []string{"--global"}
}

// commandOn is the command line of an action, run on the host. A host whose commands run in bash gets
// bashHostScript, since WSL appends Windows' PATH to its own: npx there is a Windows program under /mnt, and its
// home is the Windows profile, not the distribution's. The skill would then land where the distribution's
// sessions never look for it, and a project's install could not make its folder (cmd.exe, given a UNC folder,
// moves to C:\Windows). cmd.exe and a mapped drive give the CLI the host's own home and folder.
func commandOn(ctx context.Context, host hosts.Host, act action, inHome bool) string {
	command := commandOf(act, inHome)
	if !runsBash(ctx, host) {
		return command
	}
	return strings.ReplaceAll(bashHostScript, commandMark, command)
}

// runsBash is whether the host's commands run in bash: a Linux computer, a WSL distribution, an SSH endpoint.
func runsBash(ctx context.Context, host hosts.Host) bool {
	platform, err := host.Platform(ctx)
	return err == nil && strings.HasPrefix(platform, "linux")
}

// problemOf is why the CLI did not do what it was asked, on the host named: npx missing, its time up, or the error it
// printed. Nothing when it did it.
func problemOf(result hosts.CommandResult, act action, where string) string {
	if result.ExitCode != nil && *result.ExitCode == 0 {
		return ""
	}
	if result.ExitCode != nil && *result.ExitCode == commandNotFound {
		return fmt.Sprintf(noNpx, where)
	}
	if result.TimedOut {
		return fmt.Sprintf(cliTimedOut, act.verb, where, cliTimeout)
	}
	return fmt.Sprintf(cliFailed, act.verb, where, reasonOf(result.Output))
}

// reasonOf is the CLI's error as it printed it: its error line and the one that explains it; else the first line it
// printed, which is the shell's when npx could not start.
func reasonOf(output string) string {
	lines := linesOf(output)
	for index, line := range lines {
		if !strings.HasPrefix(line.raw, errorMark) {
			continue
		}
		return strings.Join(explained(lines[index:]), reasonSeparator)
	}
	if len(lines) == 0 {
		return ""
	}
	return lines[0].text
}

// explained is an error line and the one after it, when there is one.
func explained(lines []outputLine) []string {
	texts := []string{lines[0].text}
	if len(lines) > 1 {
		texts = append(texts, lines[1].text)
	}
	return texts
}

// outputLine is a line the CLI printed, its terminal codes taken out: as printed, to find its mark, and its text alone.
type outputLine struct {
	raw  string
	text string
}

// linesOf is the lines of the CLI's output that say something, without the terminal's codes and the CLI's frame.
func linesOf(output string) []outputLine {
	lines := []outputLine{}
	for _, line := range strings.Split(escapes.ReplaceAllString(output, ""), "\n") {
		text := strings.Trim(line, gutter)
		if text == "" {
			continue
		}
		lines = append(lines, outputLine{raw: strings.TrimSpace(line), text: text})
	}
	return lines
}

// holds is whether a folder's .agents/skills has swcode-base: the folder of its name holds a SKILL.md, as the host
// reads it. Only that folder counts, so a skill of the person's that calls itself swcode-base, in a folder of its
// own, is not taken for it.
func holds(ctx context.Context, host hosts.Host, folder string) bool {
	file := host.Paths().Join(folder, agentsFolder, skillsFolder, Name, skills.SkillFile)
	info, err := host.FileInfo(ctx, file)
	return err == nil && info.Kind == hosts.KindFile
}

// Service knows where swcode-base is, and has the skills CLI put it where a person asks: a home, or a project. Both
// are on the host the project is on, where its sessions read their skills.
type Service struct {
	hosts  *hosts.Registry
	logger *slog.Logger
}

func NewService(registry *hosts.Registry, logger *slog.Logger) *Service {
	return &Service{hosts: registry, logger: logger}
}

// hostRefOf is the host the project is on; this computer when there is no project.
func hostRefOf(project *protocol.Project) protocol.HostRef {
	if project == nil {
		return protocol.LocalHost()
	}
	return project.Host
}

// Status answers what GET /api/swcode-base says: whether swcode-base is in the home, and in the project, both read
// on the project's host (this computer's home without a project), so the app offers it when neither has it.
func (s *Service) Status(ctx context.Context, project *protocol.Project) protocol.SwcodeBaseStatus {
	host := s.hosts.HostOf(hostRefOf(project))
	home, err := host.Home(ctx)
	if err != nil {
		return failed(fmt.Sprintf(homeUnread, err))
	}
	inHome := holds(ctx, host, home)
	inProject := project != nil && holds(ctx, host, project.Path)
	return protocol.SwcodeBaseStatus{InHome: inHome, InProject: inProject, Installed: inHome || inProject}
}

// Install has the skills CLI put swcode-base in the home or in the project's .agents/skills, on the project's host
// (this computer's home without a project); then where it is.
func (s *Service) Install(ctx context.Context, project *protocol.Project, inHome bool) protocol.SwcodeBaseStatus {
	return s.run(ctx, project, inHome, adding)
}

// Remove has the skills CLI take swcode-base out of the home or out of the project, on the project's host; what the
// other place holds stays. Then where it is.
func (s *Service) Remove(ctx context.Context, project *protocol.Project, inHome bool) protocol.SwcodeBaseStatus {
	return s.run(ctx, project, inHome, removing)
}

// run has the skills CLI do an action on the project's host, in the folder it acts on, where it keeps its lock
// file too: the home, or the project. What the CLI printed goes to the log when it failed.
func (s *Service) run(ctx context.Context, project *protocol.Project, inHome bool, act action) protocol.SwcodeBaseStatus {
	if !inHome && project == nil {
		return failed(noProject)
	}
	ref := hostRefOf(project)
	host := s.hosts.HostOf(ref)
	folder, problem := folderFor(ctx, host, project, inHome)
	if problem != "" {
		return failed(problem)
	}
	command := commandOn(ctx, host, act, inHome)
	result := host.Run(ctx, command, hosts.CommandOptions{Cwd: folder, Timeout: cliTimeout})
	if problem := problemOf(result, act, whereOf(ref)); problem != "" {
		s.logger.Warn("the skills CLI failed", "command", command, "output", result.Output)
		return failed(problem)
	}
	return s.Status(ctx, project)
}

// whereOf is the host as a problem names it: this computer, a WSL distribution, an SSH endpoint.
func whereOf(ref protocol.HostRef) string {
	if hosts.IsLocal(ref) {
		return thisComputer
	}
	return hosts.Label(ref)
}

// folderFor is the folder the request means, on the project's host: its home, or the project's.
func folderFor(ctx context.Context, host hosts.Host, project *protocol.Project, inHome bool) (string, string) {
	if !inHome {
		return project.Path, ""
	}
	home, err := host.Home(ctx)
	if err != nil {
		return "", fmt.Sprintf(homeUnread, err)
	}
	return home, ""
}

// failed is the answer of a request that could not be answered: why.
func failed(problem string) protocol.SwcodeBaseStatus {
	return protocol.SwcodeBaseStatus{Problem: problem}
}
