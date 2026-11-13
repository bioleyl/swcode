package hosts

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

const (
	nul        = "\x00"
	line       = "\n"
	base64Line = 76
	// killGraceSeconds: a group that ignores SIGTERM this long is killed, as on this computer.
	killGraceSeconds = 2
	pidMarker        = "__swcode_pid "
	// The most names a listing brings back, before the glob picks among them.
	maxListed     = 200_000
	jobIDBytes    = 6
	swcodeFolder  = "$HOME/.swcode"
	scratchFolder = swcodeFolder + "/scratch"
	jobsFolder    = swcodeFolder + "/jobs"
	runningMark   = "running"
	// quietExports: commands run without a terminal, so no pager waiting for a key, no colour codes in the output.
	quietExports = "export FORCE_COLOR=0 GIT_PAGER=cat NO_COLOR=1 PAGER=cat"
	msPerSecond  = 1000
)

var ignoredFolderNames = []string{
	".cache", ".git", ".next", ".venv", "__pycache__", "build", "coverage", "dist", "node_modules", "target", "venv",
}

// decoded is a word the script decodes there: any text, quotes and newlines included, as base64.
func decoded(text string) string {
	return `"$(printf %s ` + base64.StdEncoding.EncodeToString([]byte(text)) + ` | base64 -d)"`
}

func fromBase64(text string) string {
	data, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	return string(data)
}

func wrapped(encoded string) string {
	lines := []string{}
	for start := 0; start < len(encoded); start += base64Line {
		lines = append(lines, encoded[start:min(len(encoded), start+base64Line)])
	}
	return strings.Join(lines, line)
}

func randomID() string {
	id := make([]byte, jobIDBytes)
	_, _ = rand.Read(id)
	return hex.EncodeToString(id)
}

type shellJob struct {
	command string
	// offset is how much of its log was read.
	offset int
	// dropped is output dropped unread, the log having grown past the limit, since the last read.
	dropped int
}

// ShellHost is a host reached through a bash there, with the tools any Linux has (stat, base64, find, grep, git): a
// WSL distribution through wsl.exe, an SSH endpoint through ssh. Quick requests go to one shell kept open;
// commands run in processes of their own, each in a process group the shell can stop; background jobs keep their
// output in files under ~/.swcode/jobs there.
type ShellHost struct {
	transport ShellTransport
	session   *ShellSession
	mu        sync.Mutex
	jobs      map[string]shellJob
	home      string
	platform  string
}

func NewShellHost(transport ShellTransport) *ShellHost {
	return &ShellHost{jobs: map[string]shellJob{}, session: NewShellSession(transport), transport: transport}
}

func (h *ShellHost) Paths() Paths { return PosixPaths }

func (h *ShellHost) remembered(ctx context.Context, slot *string, script string, shape func(string) string) (string, error) {
	h.mu.Lock()
	known := *slot
	h.mu.Unlock()
	if known != "" {
		return known, nil
	}
	answer, err := h.session.Run(ctx, script)
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	*slot = shape(answer)
	h.mu.Unlock()
	return shape(answer), nil
}

func (h *ShellHost) Platform(ctx context.Context) (string, error) {
	return h.remembered(ctx, &h.platform, "uname -s", func(answer string) string {
		return strings.ToLower(strings.TrimSpace(answer))
	})
}

func (h *ShellHost) Home(ctx context.Context) (string, error) {
	return h.remembered(ctx, &h.home, `printf %s "$HOME"`, func(answer string) string { return answer })
}

func (h *ShellHost) FileInfo(ctx context.Context, target string) (FileInfo, error) {
	word := ShellWord(target)
	answer, err := h.session.Run(ctx, fmt.Sprintf(
		"if [ -d %s ]; then echo directory; elif [ -e %s ]; then stat -L -c 'file %%.3Y %%s' -- %s; else echo missing; fi",
		word, word, word))
	if err != nil {
		return FileInfo{}, err
	}
	fields := strings.Fields(answer)
	kind := FileKind(at(fields, 0))
	if kind == KindDirectory || kind == KindMissing {
		return FileInfo{Kind: kind}, nil
	}
	seconds, _ := strconv.ParseFloat(at(fields, 1), 64)
	size, _ := strconv.ParseInt(at(fields, 2), 10, 64)
	return FileInfo{Kind: KindFile, ModTimeMs: int64(seconds*msPerSecond + 0.5), Size: size}, nil
}

func at(items []string, index int) string {
	if index < len(items) {
		return items[index]
	}
	return ""
}

func (h *ShellHost) ReadFile(ctx context.Context, target string) ([]byte, error) {
	answer, err := h.session.Run(ctx, "base64 -w0 -- "+ShellWord(target))
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(answer))
}

// WriteFile sends the bytes in base64, in the script itself, and they come out whole there.
func (h *ShellHost) WriteFile(ctx context.Context, target string, data []byte) error {
	word := ShellWord(target)
	end := "__swcode_" + randomID()
	_, err := h.session.Run(ctx, fmt.Sprintf("mkdir -p -- \"$(dirname -- %s)\" && base64 -d > %s <<'%s'\n%s\n%s",
		word, word, end, wrapped(base64.StdEncoding.EncodeToString(data)), end))
	return err
}

func (h *ShellHost) RemoveFile(ctx context.Context, target string) error {
	_, err := h.session.Run(ctx, "rm -f -- "+ShellWord(target))
	return err
}

func (h *ShellHost) RealPath(ctx context.Context, target string) (string, error) {
	answer, err := h.session.Run(ctx, "realpath -m -- "+ShellWord(target))
	return strings.TrimSpace(answer), err
}

// ListFiles is the files under a folder that match a glob, up to a limit: in a git repository the ones git shows,
// elsewhere all but dependency, build and cache folders, as on this computer. Both come sorted, as this computer
// lists them, whatever order find walks the disk in. The glob is matched here, on the names.
func (h *ShellHost) ListFiles(ctx context.Context, base, pattern string, limit int) ([]string, error) {
	prunes := []string{}
	for _, name := range ignoredFolderNames {
		prunes = append(prunes, "-name "+ShellWord(name))
	}
	listing, err := h.session.Run(ctx, strings.Join([]string{
		fmt.Sprintf("cd -- %s 2>/dev/null || exit 0", ShellWord(base)),
		"if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then git -c core.fsmonitor=false ls-files --cached --others --exclude-standard -z",
		fmt.Sprintf(`else find . \( -type d \( %s \) -prune \) -o -type f -printf '%%P\0' | LC_ALL=C sort -z; fi | head -z -n %d | base64 -w0`,
			strings.Join(prunes, " -o "), maxListed),
	}, line))
	if err != nil {
		return nil, err
	}
	found := []string{}
	for _, relative := range strings.Split(fromBase64(listing), nul) {
		if len(found) >= limit {
			break
		}
		if relative != "" && !isIgnored(relative) && doublestar.MatchUnvalidated(pattern, relative) {
			found = append(found, path.Join(base, relative))
		}
	}
	return found, nil
}

// Search is the matching lines, found by grep there among the files the listing gives, Perl's regular expressions.
func (h *ShellHost) Search(ctx context.Context, request SearchRequest) ([]SearchMatch, error) {
	files := []string{request.Root}
	info, err := h.FileInfo(ctx, request.Root)
	if err != nil {
		return nil, err
	}
	if info.Kind != KindFile {
		if files, err = h.ListFiles(ctx, request.Root, request.Include, request.MaxFiles); err != nil {
			return nil, err
		}
	}
	if len(files) == 0 {
		return []SearchMatch{}, nil
	}
	ignoreCase := ""
	if strings.Contains(request.Flags, ignoreCaseFlag) {
		ignoreCase = "-i"
	}
	answer, err := h.session.Run(ctx, fmt.Sprintf(
		"printf %%s %s | base64 -d | xargs -0 -r grep -nIHZP %s -e %s -- 2>/dev/null | head -n %d | base64 -w0",
		base64.StdEncoding.EncodeToString([]byte(strings.Join(files, nul))), ignoreCase, decoded(request.Source),
		request.MaxMatches))
	if err != nil {
		return nil, err
	}
	matches := []SearchMatch{}
	for _, record := range strings.Split(fromBase64(answer), line) {
		file, rest, _ := strings.Cut(record, nul)
		number, text, found := strings.Cut(rest, ":")
		if found {
			lineNumber, _ := strconv.Atoi(number)
			matches = append(matches, SearchMatch{File: file, Line: lineNumber, Text: text})
		}
	}
	return matches, nil
}

// groupOutput holds the error stream until the command's group id came, first, on a line of its own; then it is
// the command's output.
type groupOutput struct {
	mu     sync.Mutex
	output *OutputBuffer
	said   []byte
	pid    string
	known  chan struct{}
}

func (g *groupOutput) Write(data []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pid != "" {
		return g.output.Write(data)
	}
	g.said = append(g.said, data...)
	end := bytes.IndexByte(g.said, '\n')
	if end < 0 || !bytes.HasPrefix(g.said, []byte(pidMarker)) {
		return len(data), nil
	}
	g.pid = strings.TrimSpace(string(g.said[len(pidMarker):end]))
	close(g.known)
	g.output.Write(g.said[end+1:])
	return len(data), nil
}

func (g *groupOutput) group() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.pid
}

// flush gives the output what the error stream said when no group id came: why the command could not start.
func (g *groupOutput) flush() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pid == "" && len(g.said) > 0 {
		g.output.Write(g.said)
	}
}

// Run runs a command in a process of its own, in its own process group: the first thing it says is that group's
// id, so the shell can stop the command and all it started when the turn stops or the time is up. The command's
// error stream joins its output, in the order it wrote them: the shell's own carries the group id.
func (h *ShellHost) Run(ctx context.Context, command string, options CommandOptions) CommandResult {
	script := strings.Join([]string{
		fmt.Sprintf("cd -- %s || exit 1", ShellWord(options.Cwd)),
		quietExports,
		fmt.Sprintf("setsid bash -c %s 2>&1 </dev/null &", decoded(command)),
		fmt.Sprintf(`printf '%s%%s\n' "$!" >&2`, pidMarker),
		`wait "$!"`,
	}, line)
	output := NewOutputBuffer(headChars, tailChars)
	errors := &groupOutput{known: make(chan struct{}), output: output}
	cmd := h.transport.Command(script)
	cmd.Stdout = heard(output, options.Live)
	cmd.Stderr = errors
	cmd.WaitDelay = exitGrace
	if err := cmd.Start(); err != nil {
		output.Write([]byte(err.Error()))
		return CommandResult{Output: output.Text()}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(options.Timeout)
	defer timer.Stop()
	stop := func() {
		if group := errors.group(); group != "" {
			h.stopGroup(group)
		}
		time.AfterFunc(killGraceSeconds*time.Second, func() { _ = cmd.Process.Kill() })
		<-done
		errors.flush()
	}
	select {
	case err := <-done:
		errors.flush()
		return CommandResult{ExitCode: exitCodeOf(err), Output: output.Text()}
	case <-ctx.Done():
		stop()
		return CommandResult{Output: output.Text(), Stopped: true}
	case <-timer.C:
		stop()
		return CommandResult{Output: output.Text(), TimedOut: true}
	}
}

// StartJob runs the command in its own session there, its output in a file, its status in another once it ends.
func (h *ShellHost) StartJob(ctx context.Context, command, cwd string) (string, error) {
	id := randomID()
	file := `"` + jobsFolder + "/" + id
	_, err := h.session.Run(ctx, strings.Join([]string{
		fmt.Sprintf(`mkdir -p "%s" && cd -- %s || exit 1`, jobsFolder, ShellWord(cwd)),
		quietExports,
		// The command in a bash of its own, so an exit in it still leaves its status to be written.
		fmt.Sprintf(`setsid bash -c 'bash -c "$1"; echo $? > "$2"' _ %s %s.exit" > %s.log" 2>&1 </dev/null &`,
			decoded(command), file, file),
		fmt.Sprintf(`echo "$!" > %s.pid"`, file),
	}, line))
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	h.jobs[id] = shellJob{command: command}
	h.mu.Unlock()
	return id, nil
}

// StartTerminal starts a shell there, in the folder, on a pseudo-terminal of its own: ssh and wsl.exe ask the far
// end for one, and this terminal keeps its size.
func (h *ShellHost) StartTerminal(options TerminalOptions) (*Terminal, error) {
	return StartTerminal(h.transport.TerminalCommand(options.Cwd), options, Bash)
}

func (h *ShellHost) ReadJob(ctx context.Context, id string, consume bool) (*JobState, error) {
	h.mu.Lock()
	job, ok := h.jobs[id]
	h.mu.Unlock()
	if !ok {
		return nil, nil
	}
	file := `"` + jobsFolder + "/" + id
	answer, err := h.session.Run(ctx, strings.Join([]string{
		fmt.Sprintf(`pid=$(cat %s.pid" 2>/dev/null)`, file),
		fmt.Sprintf(`if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then echo %s; else echo ended; fi`, runningMark),
		fmt.Sprintf(`cat %s.exit" 2>/dev/null || echo`, file),
		fmt.Sprintf(`size=$(stat -c %%s %s.log" 2>/dev/null || echo 0)`, file),
		fmt.Sprintf(`start=$(( size - %d > %d ? size - %d : %d ))`, job.offset, maxUnreadBytes, maxUnreadBytes, job.offset),
		`echo "$start"`,
		fmt.Sprintf(`tail -c +$((start + 1)) %s.log" 2>/dev/null | head -c $((size - start)) | base64 -w0`, file),
	}, line))
	if err != nil {
		return nil, err
	}
	parts := strings.Split(answer, line)
	output, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(at(parts, 3)))
	start, _ := strconv.Atoi(strings.TrimSpace(at(parts, 2)))
	dropped := job.dropped + start - job.offset
	h.mu.Lock()
	if consume {
		h.jobs[id] = shellJob{command: job.command, offset: start + len(output)}
	} else {
		h.jobs[id] = shellJob{command: job.command, dropped: dropped, offset: start}
	}
	h.mu.Unlock()
	return &JobState{
		Command:  job.command,
		Dropped:  dropped,
		ExitCode: exitOf(at(parts, 1)),
		Output:   string(output),
		Running:  strings.TrimSpace(at(parts, 0)) == runningMark,
	}, nil
}

func exitOf(text string) *int {
	code, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return nil
	}
	return &code
}

// StopJob stops a job and all it started; what it printed before is still there to read once.
func (h *ShellHost) StopJob(ctx context.Context, id string) (*JobState, error) {
	h.mu.Lock()
	_, ok := h.jobs[id]
	h.mu.Unlock()
	if !ok {
		return nil, nil
	}
	pid, _ := h.session.Run(ctx, fmt.Sprintf(`cat "%s/%s.pid" 2>/dev/null || true`, jobsFolder, id))
	if group := strings.TrimSpace(pid); group != "" {
		h.stopGroup(group)
	}
	return h.ReadJob(ctx, id, true)
}

func (h *ShellHost) ListFolders(ctx context.Context, input string) (FolderListing, error) {
	home, err := h.Home(ctx)
	if err != nil {
		return FolderListing{}, err
	}
	target, ok := posixFullPath(input, home)
	if !ok {
		return FolderListing{Path: input, Problem: fullPathHint}, nil
	}
	answer, err := h.session.Run(ctx, strings.Join([]string{
		fmt.Sprintf("cd -- %s 2>/dev/null || { echo missing; exit 0; }", ShellWord(target)),
		"echo ok",
		"pwd -P",
		fmt.Sprintf(`find . -mindepth 1 -maxdepth 1 -type d ! -name '%s*' -printf '%%f\0' | base64 -w0`, hiddenPrefix),
	}, line))
	if err != nil {
		return FolderListing{}, err
	}
	parts := strings.Split(answer, line)
	if at(parts, 0) != "ok" {
		return FolderListing{Path: input, Problem: fmt.Sprintf("%s is not a folder on %s.", input, h.transport.Name())}, nil
	}
	folders := []string{}
	for _, name := range strings.Split(fromBase64(at(parts, 2)), nul) {
		if name != "" {
			folders = append(folders, name)
		}
	}
	sort.Strings(folders)
	real := at(parts, 1)
	parent := path.Dir(real)
	if parent == real {
		parent = ""
	}
	return FolderListing{Folders: folders, OK: true, Parent: parent, Path: real}, nil
}

func (h *ShellHost) MakeFolder(ctx context.Context, parentInput, name string) (FolderMaking, error) {
	home, err := h.Home(ctx)
	if err != nil {
		return FolderMaking{}, err
	}
	parent, ok := posixFullPath(parentInput, home)
	notFolder := FolderMaking{Problem: fmt.Sprintf("%s is not a folder on %s.", parentInput, h.transport.Name())}
	if !ok {
		return notFolder, nil
	}
	if info, err := h.FileInfo(ctx, parent); err != nil || info.Kind != KindDirectory {
		return notFolder, err
	}
	if relativeNames[name] {
		return FolderMaking{Problem: fmt.Sprintf("%s cannot be a folder's name.", name)}, nil
	}
	real, err := h.session.Run(ctx, fmt.Sprintf("cd -- %s && pwd -P", ShellWord(parent)))
	if err != nil {
		return FolderMaking{}, err
	}
	target := path.Join(strings.TrimSpace(real), name)
	if _, err := h.session.Run(ctx, "mkdir -- "+ShellWord(target)); err != nil {
		reason := err.Error()
		if strings.Contains(reason, "exists") {
			reason = "it already exists"
		}
		return FolderMaking{Problem: fmt.Sprintf("%s cannot be made: %s.", target, reason)}, nil
	}
	return FolderMaking{OK: true, Path: target}, nil
}

func (h *ShellHost) RemoveFolder(ctx context.Context, target string) error {
	_, err := h.session.Run(ctx, "rm -rf -- "+ShellWord(target))
	return err
}

func (h *ShellHost) ScratchFolder(ctx context.Context, sessionID string) (string, error) {
	folder := scratchFolder + "/" + sessionID
	answer, err := h.session.Run(ctx, fmt.Sprintf(`mkdir -p "%s" && printf %%s "%s"`, folder, folder))
	return strings.TrimSpace(answer), err
}

func (h *ShellHost) RemoveScratch(ctx context.Context, sessionID string) error {
	_, err := h.session.Run(ctx, fmt.Sprintf(`rm -rf -- "%s/%s"`, scratchFolder, sessionID))
	return err
}

// Close stops the jobs the daemon started there, rather than let them run on unseen, and ends the shell.
func (h *ShellHost) Close() {
	h.mu.Lock()
	lines := []string{}
	for id := range h.jobs {
		lines = append(lines, fmt.Sprintf(`pid=$(cat "%s/%s.pid" 2>/dev/null) && kill -TERM -- -"$pid" 2>/dev/null`, jobsFolder, id))
	}
	h.mu.Unlock()
	if len(lines) > 0 {
		_, _ = h.session.Run(context.Background(), strings.Join(append(lines, "true"), line))
	}
	h.session.Close()
}

// stopGroup ends a process group there: politely, then for good a moment later.
func (h *ShellHost) stopGroup(pid string) {
	_, _ = h.session.Run(context.Background(), fmt.Sprintf(
		"kill -TERM -- -%s 2>/dev/null; ( sleep %d; kill -KILL -- -%s 2>/dev/null ) >/dev/null 2>&1 &",
		pid, killGraceSeconds, pid))
}

// posixFullPath is a path given as full or from ~, as the folder browser takes them.
func posixFullPath(input, home string) (string, bool) {
	expanded := input
	if input == homeMark || strings.HasPrefix(input, homeMark+"/") {
		expanded = path.Join(home, input[1:])
	}
	if !path.IsAbs(expanded) {
		return "", false
	}
	return path.Clean(expanded), true
}
