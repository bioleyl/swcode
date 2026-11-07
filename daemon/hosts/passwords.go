package hosts

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	// lookEvery: how often a running command's output is looked at, to tell when it went quiet.
	lookEvery = 100 * time.Millisecond
	// quietWait: the output is read for a password prompt once it has stayed this long without a word; a program
	// that asks for one writes nothing more until it has the answer.
	quietWait = 700 * time.Millisecond
	// A program asks for a password in a line of its own: only the end of the output is read for it.
	maxPromptChars = 160
	// The most a command is asked for a password in one call: sudo's and ssh's three tries. Past it, its input is
	// closed.
	maxAsks = 3
	// A secret longer than this was not typed at a card: the command gets an empty line instead.
	maxSecretBytes = 512
	// Where a prompt starts, and the words it asks with: `Password: `, `[sudo] password for leal: `,
	// `Enter passphrase for key '/home/me/.ssh/id_ed25519': `.
	passwordWords = `(?i)(?:password|passphrase)`
	// A prompt note: what the daemon's password prompt writes just before the words it asks with, in a form no person
	// types and no program prints. Whatever follows it on its line is a password prompt, in whatever language sudo
	// speaks.
	promptNote = "\x00swcode:ask\x00"
	// What a prompt note reads as for the person and the model.
	promptShown = "\n"
	// What the person and the model are told when no password was given: the command reads no more, and its own words
	// follow.
	noPasswordGiven = "[no password given: the command's input was closed]\n"
	// The daemon's password prompt, in its helpers folder; the folder is the daemon's user's only.
	askpassFile = "askpass"
	helpersMode = 0o700
	windowsOS   = "windows"
)

// askpassScript is the program sudo -A and ssh run for a password instead of reading one from a terminal: it writes
// the prompt note and the words it was given to the command's output, and hands back the line the daemon writes to
// the command's input; its standard output is the answer, read by sudo or ssh. What is not a password is answered at
// once (ssh's yes-or-no questions with a no, its notices with nothing), never waiting on a person who is not asked.
// The note is printf's `\000`, the same bytes as promptNote.
const askpassScript = `#!/bin/sh
case "$SSH_ASKPASS_PROMPT" in
confirm) exit 1 ;;
none) exit 0 ;;
esac
case "$1" in
*yes/no*) exit 1 ;;
esac
printf '\000swcode:ask\000%s' "${1:-Password: }" >&2
IFS= read -r answer || exit 1
printf '%s\n' "$answer"
`

var (
	// promptWords is a line that asks for a password, alone at the end of what a command printed.
	promptWords = regexp.MustCompile(`[^\r\n]*` + passwordWords + `[^\r\n:]*:`)
	// notesShown shows a prompt note as the line break it stands for, and as nothing where a line starts already.
	notesShown = strings.NewReplacer("\n"+promptNote, "\n", promptNote, promptShown)
)

// askpassEnvironment points sudo -A and ssh at the daemon's password prompt; ssh then asks through it even where it
// could read a terminal.
func askpassEnvironment(askpass string) []string {
	if askpass == "" {
		return nil
	}
	return []string{"SSH_ASKPASS=" + askpass, "SSH_ASKPASS_REQUIRE=force", "SUDO_ASKPASS=" + askpass}
}

// WriteAskpass writes the daemon's password prompt into a folder and is its path; empty when it cannot be written,
// or on Windows, whose sudo and ssh do not run a shell script. It is written whole under another name first, so a
// command running the one there never reads half of it.
func WriteAskpass(folder string) string {
	if folder == "" || runtime.GOOS == windowsOS {
		return ""
	}
	if err := os.MkdirAll(folder, helpersMode); err != nil {
		return ""
	}
	draft, err := os.CreateTemp(folder, askpassFile+"-*")
	if err != nil {
		return ""
	}
	defer func() { _ = os.Remove(draft.Name()) }()
	_, writeErr := draft.WriteString(askpassScript)
	closeErr := draft.Close()
	path := filepath.Join(folder, askpassFile)
	if writeErr != nil || closeErr != nil || os.Chmod(draft.Name(), helpersMode) != nil || os.Rename(draft.Name(), path) != nil {
		return ""
	}
	return path
}

// PromptOf is the words a command asked a password with, when what it printed ends on them: what the daemon's own
// prompt asked, else a last line that asks for a password; empty when it does not, so nothing is asked and the
// command is left to its time.
func PromptOf(output string) string {
	tail := output[max(0, len(output)-maxPromptChars):]
	if start := strings.LastIndex(tail, promptNote); start >= 0 && !strings.ContainsAny(tail[start:], "\r\n") {
		// The daemon's own prompt asked, and nothing came after: its words are a prompt in any language.
		return strings.TrimSpace(tail[start+len(promptNote):])
	}
	return asked(tail[strings.LastIndexAny(tail, "\r\n")+1:])
}

// asked is a line, when it asks for a password; empty when it does not.
func asked(line string) string {
	if promptWords.MatchString(line) {
		return strings.TrimSpace(line)
	}
	return ""
}

// withoutNotes is what a command printed, its prompt notes shown as the line breaks they stand for.
func withoutNotes(output string) string {
	return notesShown.Replace(strings.TrimPrefix(output, promptNote))
}

// prompter is a command's input, and what it asked for while it ran: whenever its output went quiet on a password
// prompt not asked about yet, the person is asked, not more than maxAsks times.
type prompter struct {
	given     io.WriteCloser
	output    *OutputBuffer
	passwords Passwords
	ticker    *time.Ticker
	// seen is how much the command had printed at the last look, and quietFrom since when it printed nothing more.
	seen      int
	quietFrom time.Time
	// read is how much it had printed when its output was last read for a prompt.
	read int
	// times is how many times the person was asked already.
	times int
	// waiting is while the person is asked, over once nothing more is asked: no password given, or the last one.
	waiting bool
	over    bool
	// replies speaks when the person answered: true when the command's input is closed since.
	replies chan bool
}

// prompterOf gives a command an input and the daemon's password prompt, and watches it; nil where nobody can answer
// what it asks for, and the command then has no input at all.
func prompterOf(cmd *exec.Cmd, output *OutputBuffer, passwords Passwords, askpass string) *prompter {
	if passwords == nil {
		return nil
	}
	given, err := cmd.StdinPipe()
	if err != nil {
		return nil
	}
	cmd.Env = append(cmd.Env, askpassEnvironment(askpass)...)
	return &prompter{
		given: given, output: output, passwords: passwords, quietFrom: time.Now(), replies: make(chan bool, 1),
		ticker: time.NewTicker(lookEvery),
	}
}

// looks speaks each time the output is worth looking at; nil where there is no one to answer, while the person is
// asked, and once nothing more is asked.
func (p *prompter) looks() <-chan time.Time {
	if p == nil || p.waiting || p.over {
		return nil
	}
	return p.ticker.C
}

// answered speaks when the person answered what the command asked for; nil where there is no one to answer.
func (p *prompter) answered() <-chan bool {
	if p == nil {
		return nil
	}
	return p.replies
}

// look reads the output for a password prompt once it went quiet after printing something not read yet, and asks
// the person when it ends on one; whether it asked. The answer comes on replies.
func (p *prompter) look(ctx context.Context) bool {
	now, written := time.Now(), p.output.Written()
	if written != p.seen {
		p.seen, p.quietFrom = written, now
		return false
	}
	if written == p.read || now.Sub(p.quietFrom) < quietWait {
		return false
	}
	p.read = written
	words := PromptOf(p.output.Text())
	if words == "" {
		return false
	}
	p.times, p.waiting = p.times+1, true
	go p.ask(ctx, words, p.times >= maxAsks)
	return true
}

// replied is the person's answer come back to the command's loop: the output is looked at again, unless nothing
// more is asked.
func (p *prompter) replied(closed bool) {
	p.waiting, p.over = false, closed
}

// ask asks the person for the password the command stopped on, and hands it their answer. Once the command is over
// (ctx done), nothing is handed: its loop is gone.
func (p *prompter) ask(ctx context.Context, words string, last bool) {
	text, gave := p.passwords.Secret(ctx, words)
	if ctx.Err() != nil {
		return
	}
	p.replies <- p.give(text, gave, last)
}

// give writes the password to the command's input, its line in the output ending there as on a terminal, and is
// whether the input is closed since: after no password, its output saying so, and after the last one. A closed input
// makes the command's next read find nothing, so it fails at once instead of waiting out its time.
func (p *prompter) give(text string, gave, last bool) bool {
	if !gave {
		p.output.Write([]byte(noPasswordGiven))
		_ = p.given.Close()
		return true
	}
	p.output.Write([]byte(promptShown))
	_, _ = io.WriteString(p.given, lineOf(text))
	if !last {
		return false
	}
	_ = p.given.Close()
	return true
}

// lineOf is the line a command reads for a password: its first line, or an empty one when it was not typed at a card.
func lineOf(text string) string {
	first, _, _ := strings.Cut(text, "\n")
	if len(first) > maxSecretBytes {
		return "\n"
	}
	return strings.TrimSuffix(first, "\r") + "\n"
}

// close lets go of a command's input and its looks once it is over.
func (p *prompter) close() {
	if p == nil {
		return
	}
	p.ticker.Stop()
	_ = p.given.Close()
}
