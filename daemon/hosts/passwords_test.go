package hosts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// typed is a person at the card: each time they are asked they type the next of their passwords (the last one again
// once there are no more), after a moment; with none, they give none.
type typed struct {
	answers []string
	delay   time.Duration
	mu      sync.Mutex
	asked   []string
}

func (p *typed) Secret(ctx context.Context, prompt string) (string, bool) {
	p.mu.Lock()
	p.asked = append(p.asked, prompt)
	times := len(p.asked)
	p.mu.Unlock()
	select {
	case <-time.After(p.delay):
	case <-ctx.Done():
		return "", false
	}
	if len(p.answers) == 0 {
		return "", false
	}
	return p.answers[min(times, len(p.answers))-1], true
}

func (p *typed) prompts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string{}, p.asked...)
}

const (
	commandTime = 20 * time.Second
	// atOnce: a command that does not wait out its time ends well before it.
	atOnce = 8 * time.Second
)

// testShell is what the commands of these tests run with: bash, Git Bash on Windows.
func testShell() Shell {
	if runtime.GOOS != windowsOS {
		return Bash
	}
	environment := map[string]string{}
	for _, entry := range os.Environ() {
		if name, value, ok := strings.Cut(entry, "="); ok {
			environment[name] = value
		}
	}
	return WindowsShell(environment)
}

// withAskpass skips a test where the daemon has no password prompt for sudo and ssh (Windows).
func withAskpass(t *testing.T) {
	t.Helper()
	if WriteAskpass(t.TempDir()) == "" {
		t.Skip("no password prompt for sudo and ssh here")
	}
}

// runAsking runs a command of this computer whose passwords the person types, with the daemon's password prompt
// where it has one.
func runAsking(t *testing.T, command string, person *typed, timeout time.Duration) CommandResult {
	t.Helper()
	folder := t.TempDir()
	options := CommandOptions{Cwd: folder, Passwords: person, Timeout: timeout}
	return RunCommandAsks(context.Background(), command, options, testShell(), WriteAskpass(filepath.Join(folder, "helpers")))
}

func TestPromptOfIsWhatTheDaemonsPromptAskedOrALastLineAskingForAPassword(t *testing.T) {
	for _, test := range []struct{ name, output, prompt string }{
		{"a program's own prompt", "Connecting\nPassword: ", "Password:"},
		{"sudo's", "[sudo] password for leal: ", "[sudo] password for leal:"},
		{"a key's passphrase", "Enter passphrase for key '/home/me/.ssh/id_ed25519': ", "Enter passphrase for key '/home/me/.ssh/id_ed25519':"},
		{"a line that went on", "Password: ok\n", ""},
		{"a question that is no password", "Your name: ", ""},
		{"the daemon's prompt, in another language", "x" + promptNote + "[sudo] Mot de passe de leal : ", "[sudo] Mot de passe de leal :"},
		{"the daemon's prompt, answered and gone on", promptNote + "[sudo] Mot de passe de leal : \nDone", ""},
		{"a program's prompt after the daemon's", promptNote + "[sudo] Mot de passe : \nPassword: ", "Password:"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := PromptOf(test.output); got != test.prompt {
				t.Fatalf("PromptOf(%q) = %q, want %q", test.output, got, test.prompt)
			}
		})
	}
}

func TestThePromptsNotesReadAsTheLinesTheyStart(t *testing.T) {
	output := promptNote + "[sudo] password for me: \nSorry, try again.\n" + promptNote + "[sudo] password for me: x" + promptNote + "Password: "
	want := "[sudo] password for me: \nSorry, try again.\n[sudo] password for me: x\nPassword: "
	if got := withoutNotes(output); got != want {
		t.Fatalf("withoutNotes = %q, want %q", got, want)
	}
}

// TestAPasswordGoesToTheCommandAndNeverToItsOutput: its prompt's line ends where it was typed, as on a terminal.
func TestAPasswordGoesToTheCommandAndNeverToItsOutput(t *testing.T) {
	person := &typed{answers: []string{"hunter2"}}
	result := runAsking(t, `printf 'Password: '; read -r line; echo "got ${#line} characters"`, person, commandTime)
	if !strings.Contains(result.Output, "Password: \ngot 7 characters") || strings.Contains(result.Output, "hunter2") ||
		len(person.prompts()) != 1 {
		t.Fatalf("output %q, asked %q", result.Output, person.prompts())
	}
}

// TestAQuestionThatIsNoPasswordIsNotAsked: the command is left to its time, as without anyone to answer.
func TestAQuestionThatIsNoPasswordIsNotAsked(t *testing.T) {
	person := &typed{answers: []string{"Ada"}}
	result := runAsking(t, `printf 'Your name: '; read -r name; echo "hello $name"`, person, 1500*time.Millisecond)
	if !result.TimedOut || len(person.prompts()) != 0 {
		t.Fatalf("result %+v, asked %q", result, person.prompts())
	}
}

func TestAPromptThatComesLongAfterTheStartIsAskedToo(t *testing.T) {
	person := &typed{answers: []string{"hunter2"}}
	result := runAsking(t, `echo connecting; sleep 1.5; printf 'Password: '; read -r line; echo "got $line"`, person, commandTime)
	if !strings.Contains(result.Output, "got hunter2") || len(person.prompts()) != 1 {
		t.Fatalf("output %q, asked %q", result.Output, person.prompts())
	}
}

func TestAnAnsweredPromptIsNotAskedAgainWhileTheCommandStaysQuiet(t *testing.T) {
	person := &typed{answers: []string{"hunter2"}}
	result := runAsking(t, `printf 'Password: '; read -r line; sleep 2; echo done`, person, commandTime)
	if !strings.Contains(result.Output, "done") || len(person.prompts()) != 1 {
		t.Fatalf("output %q, asked %q", result.Output, person.prompts())
	}
}

// TestPastItsLastTryTheCommandsNextReadFindsNothing: the person is asked maxAsks times, then the command's input is
// closed, so it does not wait out its time.
func TestPastItsLastTryTheCommandsNextReadFindsNothing(t *testing.T) {
	person := &typed{answers: []string{"wrong"}}
	started := time.Now()
	result := runAsking(t, `for try in 1 2 3 4; do
  printf 'Password: '; read -r line || echo "nothing at $try"
  echo 'Sorry, try again.'
done`, person, commandTime)
	if time.Since(started) > atOnce || !strings.Contains(result.Output, "nothing at 4") || len(person.prompts()) != maxAsks {
		t.Fatalf("after %v: output %q, asked %d times", time.Since(started), result.Output, len(person.prompts()))
	}
}

// TestNoPasswordClosesTheCommandsInput: it reads nothing from then on, fails as it did without an input, and its
// output says why; it is not asked again.
func TestNoPasswordClosesTheCommandsInput(t *testing.T) {
	person := &typed{}
	started := time.Now()
	result := runAsking(t, `printf 'Password: '; read -r line || echo "no password"
printf 'Password: '; read -r line || echo "still none"; echo after`, person, commandTime)
	if time.Since(started) > atOnce || !strings.Contains(result.Output, noPasswordGiven+"no password") ||
		!strings.Contains(result.Output, "still none") || len(person.prompts()) != 1 {
		t.Fatalf("after %v: output %q, asked %q", time.Since(started), result.Output, person.prompts())
	}
}

func TestWaitingForThePersonTakesNoneOfTheCommandsTime(t *testing.T) {
	person := &typed{answers: []string{"hunter2"}, delay: 2 * time.Second}
	result := runAsking(t, `printf 'Password: '; read -r line; echo "got $line"`, person, 1500*time.Millisecond)
	if result.TimedOut || !strings.Contains(result.Output, "got hunter2") {
		t.Fatalf("result %+v", result)
	}
}

// TestSudoAndSshRunTheDaemonsPromptWhichHandsBackWhatThePersonTyped: the prompt is a program sudo -A and ssh can
// run, its words go to the person in any language, and what they typed comes out of it, never into the output.
func TestSudoAndSshRunTheDaemonsPromptWhichHandsBackWhatThePersonTyped(t *testing.T) {
	withAskpass(t)
	person := &typed{answers: []string{"hunter2"}}
	result := runAsking(t, `[ "$SSH_ASKPASS" = "$SUDO_ASKPASS" ] && [ "$SSH_ASKPASS_REQUIRE" = force ] &&
[ "$("$SUDO_ASKPASS" '[sudo] Mot de passe de moi : ')" = hunter2 ] && echo right`, person, commandTime)
	asked := person.prompts()
	if !strings.Contains(result.Output, "right") || strings.Contains(result.Output, "hunter2") ||
		len(asked) != 1 || asked[0] != "[sudo] Mot de passe de moi :" {
		t.Fatalf("output %q, asked %q", result.Output, asked)
	}
}

// TestSshsQuestionsThatAreNoPasswordAreAnsweredAtOnce: an unknown host's yes-or-no gets a no, as without a terminal,
// and a key's confirmation is refused; the person is not asked.
func TestSshsQuestionsThatAreNoPasswordAreAnsweredAtOnce(t *testing.T) {
	withAskpass(t)
	person := &typed{answers: []string{"yes"}}
	result := runAsking(t, `"$SSH_ASKPASS" 'Are you sure you want to continue connecting (yes/no/[fingerprint])? ' || echo refused
SSH_ASKPASS_PROMPT=confirm "$SSH_ASKPASS" 'Allow use of key?' || echo denied`, person, commandTime)
	if !strings.Contains(result.Output, "refused") || !strings.Contains(result.Output, "denied") || len(person.prompts()) != 0 {
		t.Fatalf("output %q, asked %q", result.Output, person.prompts())
	}
}

// TestAKeysPassphraseReachesSshKeygen: ssh's own tools ask through the daemon's prompt, with no terminal.
func TestAKeysPassphraseReachesSshKeygen(t *testing.T) {
	withAskpass(t)
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("no ssh-keygen here")
	}
	key := filepath.Join(t.TempDir(), "key")
	if out, err := exec.Command(keygen, "-q", "-t", "ed25519", "-N", "open sesame", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	person := &typed{answers: []string{"open sesame"}}
	result := runAsking(t, "ssh-keygen -y -f "+ShellWord(key), person, commandTime)
	if !strings.Contains(result.Output, "ssh-ed25519") || len(person.prompts()) != 1 {
		t.Fatalf("output %q, asked %q", result.Output, person.prompts())
	}
}
