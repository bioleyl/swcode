package hosts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	markerBytes     = 8
	maxProblemChars = 2000
	// A request that takes this long has the shell stuck: it is ended, and the next request starts another.
	DefaultRequestLimit = 120 * time.Second
	statusOK            = "0"
)

// shellProcess is one bash kept open there, what it printed so far, and whether it ended.
type shellProcess struct {
	stdin  io.WriteCloser
	stop   func()
	mu     sync.Mutex
	output []byte
	said   string
	notify chan struct{}
	done   chan struct{}
	reason string
}

func (p *shellProcess) Write(data []byte) (int, error) {
	p.mu.Lock()
	p.output = append(p.output, data...)
	p.mu.Unlock()
	select {
	case p.notify <- struct{}{}:
	default:
	}
	return len(data), nil
}

// stderr keeps the end of what the shell said on its error stream: why it ended, when it does.
type stderr struct{ process *shellProcess }

func (s stderr) Write(data []byte) (int, error) {
	s.process.mu.Lock()
	defer s.process.mu.Unlock()
	said := s.process.said + string(data)
	s.process.said = said[max(0, len(said)-maxProblemChars):]
	return len(data), nil
}

// answer is the request's output once its second marker came: what came before its first is the login's noise.
func (p *shellProcess) answer(start string, end *regexp.Regexp) (string, string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	found := end.FindSubmatchIndex(p.output)
	if found == nil {
		return "", "", false
	}
	text := string(p.output[:found[0]])
	status := string(p.output[found[2]:found[3]])
	p.output = append([]byte(nil), p.output[found[1]:]...)
	if begin := strings.Index(text, start); begin >= 0 {
		text = text[begin+len(start):]
	}
	return text, status, true
}

func (p *shellProcess) ended() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return errors.New(firstOf(strings.TrimSpace(p.said), p.reason))
}

func firstOf(texts ...string) string {
	for _, text := range texts {
		if text != "" {
			return text
		}
	}
	return ""
}

// ShellSession is one bash kept open on a host, for the quick requests: each script runs in a subshell of its own,
// its output, errors included, between two markers made for it; its exit status comes with the second. One request
// at a time. The shell starts on the first request, and again after it ended.
type ShellSession struct {
	transport ShellTransport
	requests  sync.Mutex
	mu        sync.Mutex
	process   *shellProcess
}

func NewShellSession(transport ShellTransport) *ShellSession {
	return &ShellSession{transport: transport}
}

// Run is a script's output, once it ran; it fails with its output when it ends with a status other than 0.
func (s *ShellSession) Run(ctx context.Context, script string) (string, error) {
	return s.RunWithin(ctx, script, DefaultRequestLimit)
}

// RunWithin runs a script, the shell ended when it takes longer than limit.
func (s *ShellSession) RunWithin(ctx context.Context, script string, limit time.Duration) (string, error) {
	s.requests.Lock()
	defer s.requests.Unlock()
	process, err := s.shell()
	if err != nil {
		return "", fmt.Errorf("%s: %w", s.transport.Name(), err)
	}
	marker := make([]byte, markerBytes)
	_, _ = rand.Read(marker)
	start := "__swcode_" + hex.EncodeToString(marker) + "_start"
	end := "__swcode_" + hex.EncodeToString(marker) + "_end"
	// A subshell: what a script does (exit, cd, export) ends with it, the shell stays as it was.
	request := fmt.Sprintf("printf '%%s\\n' '%s'\n(\n%s\n) </dev/null 2>&1\nprintf '\\n%%s %%d\\n' '%s' \"$?\"\n", start, script, end)
	if _, err := io.WriteString(process.stdin, request); err != nil {
		return "", fmt.Errorf("%s: %w", s.transport.Name(), err)
	}
	return s.wait(ctx, process, start+"\n", regexp.MustCompile(`\n`+regexp.QuoteMeta(end)+` (\d+)\n`), limit)
}

func (s *ShellSession) wait(ctx context.Context, process *shellProcess, start string, end *regexp.Regexp, limit time.Duration) (string, error) {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	for {
		if output, status, ok := process.answer(start, end); ok {
			return requestOutcome(s.transport.Name(), output, status)
		}
		select {
		case <-process.notify:
		case <-process.done:
			if output, status, ok := process.answer(start, end); ok {
				return requestOutcome(s.transport.Name(), output, status)
			}
			return "", fmt.Errorf("%s: %w", s.transport.Name(), process.ended())
		case <-timer.C:
			s.Close()
			return "", fmt.Errorf("%s did not answer in %d s.", s.transport.Name(), int(limit.Seconds()))
		case <-ctx.Done():
			s.Close()
			return "", ctx.Err()
		}
	}
}

func requestOutcome(name, output, status string) (string, error) {
	if status == statusOK {
		return output, nil
	}
	return "", errors.New(firstOf(strings.TrimSpace(output), fmt.Sprintf("%s: a request failed (status %s)", name, status)))
}

// shell is the bash kept open, started when there is none.
func (s *ShellSession) shell() (*shellProcess, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.process != nil {
		select {
		case <-s.process.done:
		default:
			return s.process, nil
		}
	}
	cmd := s.transport.Command("exec bash")
	process := &shellProcess{done: make(chan struct{}), notify: make(chan struct{}, 1)}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = process
	cmd.Stderr = stderr{process: process}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	process.stdin = stdin
	process.stop = func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
	}
	go func() {
		err := cmd.Wait()
		process.mu.Lock()
		process.reason = fmt.Sprintf("its shell ended (%v)", firstOf(errText(err), "exit code 0"))
		process.mu.Unlock()
		close(process.done)
	}()
	s.process = process
	return process, nil
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Close ends the shell; the next request starts another.
func (s *ShellSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.process != nil {
		s.process.stop()
		s.process = nil
	}
}
