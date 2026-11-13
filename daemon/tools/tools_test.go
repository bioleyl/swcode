package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"swcode/daemon/browser"
	"swcode/daemon/hosts"
	"swcode/daemon/images"
	"swcode/daemon/protocol"
	"swcode/daemon/web"
)

type fixture struct {
	t         *testing.T
	workspace string
	tc        *Context
}

func setUp(t *testing.T) *fixture {
	workspace := t.TempDir()
	host := hosts.NewLocalHost(hosts.LocalOptions{Home: workspace, ScratchRoot: filepath.Join(workspace, ".scratch")})
	t.Cleanup(host.Close)
	return &fixture{t: t, tc: &Context{CallID: "call-1", Host: host, ReadFiles: NewReadFiles(), Workspace: workspace}, workspace: workspace}
}

func (f *fixture) put(path, text string) {
	f.t.Helper()
	target := filepath.Join(f.workspace, path)
	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	if err := os.WriteFile(target, []byte(text), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) read(path string) string {
	data, _ := os.ReadFile(filepath.Join(f.workspace, path))
	return string(data)
}

type result struct {
	Outcome
	refused string
}

// call prepares and runs a call as the agent loop does, without anyone to ask: the input as the model writes it.
func (f *fixture) call(name, input string) result {
	f.t.Helper()
	// Any tool, a chat's too: which agent has it is the boxes' business.
	tool, ok := all[protocol.ToolName(name)]
	if !ok {
		f.t.Fatalf("no tool %s", name)
	}
	var raw any
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		f.t.Fatal(err)
	}
	prepared := tool.Prepare(context.Background(), raw, f.tc)
	if !prepared.OK {
		return result{refused: prepared.Problem}
	}
	return result{Outcome: prepared.Call.Run(context.Background())}
}

func (f *fixture) refused(name, input string, pattern string) {
	f.t.Helper()
	if got := f.call(name, input); !regexp.MustCompile(pattern).MatchString(got.refused) {
		f.t.Fatalf("%s %s: refused %q, ran %+v", name, input, got.refused, got.Outcome)
	}
}

func (f *fixture) outputs(name, input, want string) result {
	f.t.Helper()
	got := f.call(name, input)
	if got.Output != want {
		f.t.Fatalf("%s %s: got %q (refused %q)", name, input, got.Output, got.refused)
	}
	return got
}

func TestReadNumbersTheLinesAndReadsAPartOfALongFile(t *testing.T) {
	f := setUp(t)
	f.put("a.txt", "one\ntwo\nthree\n")
	if got := f.outputs("read", `{"limit":1,"offset":2,"path":"a.txt"}`, "     2\ttwo\n[1 more line: read on with offset 3]"); got.Meta != "3 lines" {
		t.Fatalf("meta %q", got.Meta)
	}
}

func TestReadRefusesAMissingFileAPathOutsideAndInputThatDoesNotFit(t *testing.T) {
	f := setUp(t)
	f.refused("read", `{"path":"nope.txt"}`, `^nope.txt does not exist.$`)
	f.refused("read", `{"path":"../../etc/passwd"}`, `outside`)
	f.refused("read", `{"file":"a.txt"}`, `does not fit`)
}

func TestEditAfterAReadAndRefuseOnceTheFileChangedBehindTheAgentsBack(t *testing.T) {
	f := setUp(t)
	f.put("a.ts", "let x = 1;\n")
	f.call("read", `{"path":"a.ts"}`)
	if got := f.call("edit", `{"newString":"x = 2","oldString":"x = 1","path":"a.ts"}`); !got.OK {
		t.Fatalf("got %+v", got)
	}
	if f.read("a.ts") != "let x = 2;\n" {
		t.Fatalf("got %q", f.read("a.ts"))
	}
	f.put("a.ts", "let x = 3;\n")
	later := time.Now().Add(5 * time.Second)
	_ = os.Chtimes(filepath.Join(f.workspace, "a.ts"), later, later)
	f.refused("edit", `{"newString":"x = 4","oldString":"x = 3","path":"a.ts"}`, `changed since you read it`)
}

func TestWriteCreatesAFileAndItsFoldersButNeverReplacesAFileItDidNotRead(t *testing.T) {
	f := setUp(t)
	f.outputs("write", `{"content":"hi\n","path":"deep/new.txt"}`, "Created deep/new.txt (2 lines).")
	if f.read("deep/new.txt") != "hi\n" {
		t.Fatal("not written")
	}
	f.put("a.txt", "keep me")
	f.refused("write", `{"content":"gone","path":"a.txt"}`, `Read a.txt`)
}

func TestBashRunsInTheWorkspaceAndReportsAFailingExitCode(t *testing.T) {
	f := setUp(t)
	f.put("marker.txt", "")
	if got := f.outputs("bash", `{"command":"ls"}`, "marker.txt"); got.Meta != "done" || !got.OK {
		t.Fatalf("got %+v", got)
	}
	if got := f.outputs("bash", `{"command":"echo oops >&2; exit 3"}`, "oops\n[exit code 3]"); got.Meta != "exit code 3" || got.OK {
		t.Fatalf("got %+v", got)
	}
	if got := f.call("bash", `{"command":"sleep 5","timeoutMs":1000}`); got.Meta != "timed out" || got.OK {
		t.Fatalf("got %+v", got)
	}
}

func (f *fixture) sources() {
	f.put("src/a.ts", "export const answer = 42;\n")
	f.put("src/b.ts", "import { answer } from \"./a\";\n")
	f.put("node_modules/pkg/index.js", "const answer = 0;\n")
}

func TestGrepFindsTheLinesOutsideTheDependencyFolders(t *testing.T) {
	f := setUp(t)
	f.sources()
	got := f.outputs("grep", `{"include":"**/*.ts","pattern":"answer"}`, "src/a.ts:1: export const answer = 42;\nsrc/b.ts:1: import { answer } from \"./a\";")
	if got.Meta != "2 matches" {
		t.Fatalf("meta %q", got.Meta)
	}
	if got := f.call("grep", `{"pattern":"(?i)ANSWER = 42"}`); got.Meta != "1 match" {
		t.Fatalf("got %+v", got)
	}
	tool, _ := Main.Find("grep")
	if prepared := tool.Prepare(context.Background(), map[string]any{"include": "**/*.md", "path": "src", "pattern": "answer"}, f.tc); prepared.Call.Summary != `"answer" in src, **/*.md` {
		t.Fatalf("summary %q", prepared.Call.Summary)
	}
	f.refused("grep", `{"pattern":"("}`, `not a valid`)
}

func TestGlobListsTheMatchingFilesSorted(t *testing.T) {
	f := setUp(t)
	f.sources()
	if got := f.outputs("glob", `{"pattern":"**/*.{ts,js}"}`, "src/a.ts\nsrc/b.ts"); got.Meta != "2 files" {
		t.Fatalf("meta %q", got.Meta)
	}
}

func TestPlanShowsThePlanAndTellsTheModelToStopOnceEveryStepIsCompleted(t *testing.T) {
	f := setUp(t)
	steps := `[{"content":"Profile the conversion","status":"completed"},{"content":"Fix the tag lookup","status":"in_progress"}]`
	got := f.call("plan", `{"steps":`+steps+`}`)
	plan, _ := got.Display.Value.(protocol.ToolDisplayPlan)
	if got.Meta != "1 of 2 done" || len(plan.Steps) != 2 || !strings.Contains(got.Output, "[x] Profile the conversion\n[>] Fix the tag lookup") {
		t.Fatalf("got %+v", got)
	}
	done := strings.ReplaceAll(steps, "in_progress", "completed")
	if got := f.call("plan", `{"steps":`+done+`}`); !strings.Contains(got.Output, "give your answer now, and stop") {
		t.Fatalf("got %q", got.Output)
	}
	f.refused("plan", `{"steps":[]}`, `does not fit`)
	f.refused("plan", `{"steps":[{"content":"Fix","status":"started"}]}`, `does not fit`)
	if _, found := Explore.Find("plan"); found {
		t.Fatal("subagents plan")
	}
}

func TestQuestionIsRefusedWhereNoOneCanAnswer(t *testing.T) {
	f := setUp(t)
	if _, found := Explore.Find("question"); found {
		t.Fatal("subagents ask")
	}
	f.refused("question", `{"questions":[{"header":"Approach","options":[{"description":"","label":"A"},{"description":"","label":"B"}],"question":"A or B?"}]}`, `No one can be asked`)
}

func TestAPathLeadingOutsideTheWorkspaceThroughALinkIsRefused(t *testing.T) {
	f := setUp(t)
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644)
	_ = os.Symlink(outside, filepath.Join(f.workspace, "escape"))
	f.refused("read", `{"path":"escape/secret.txt"}`, `through a link`)
	f.refused("write", `{"content":"x","path":"escape/new.txt"}`, `through a link`)
}

func TestTheFileToolsGoIntoTheSessionsScratchFolderToo(t *testing.T) {
	f := setUp(t)
	f.tc.Scratch = t.TempDir()
	probe := filepath.Join(f.tc.Scratch, "probe.go")
	if got := f.call("write", `{"content":"package main\n","path":"`+probe+`"}`); !got.OK {
		t.Fatalf("got %+v", got)
	}
	f.outputs("read", `{"path":"`+probe+`"}`, "     1\tpackage main")
}

func TestOnlyWhatGitShowsIsSearchedInARepository(t *testing.T) {
	f := setUp(t)
	if output, err := exec.Command("git", "init", "-q", f.workspace).CombinedOutput(); err != nil {
		t.Fatalf("%s", output)
	}
	f.put(".gitignore", "generated/\n*.log\n")
	f.put("src/kept.ts", "const needle = 1;\n")
	f.put("generated/out.ts", "const needle = 2;\n")
	f.put("debug.log", "needle\n")
	f.outputs("grep", `{"pattern":"needle"}`, "src/kept.ts:1: const needle = 1;")
	f.outputs("glob", `{"pattern":"**/*.ts"}`, "src/kept.ts")
}

func TestARepositorysOwnFolderIsNotChangedByAFileToolAndSaysNothingToRunWhenFilesAreListed(t *testing.T) {
	f := setUp(t)
	if output, err := exec.Command("git", "init", "-q", f.workspace).CombinedOutput(); err != nil {
		t.Fatalf("%s", output)
	}
	// Its settings and hooks say what git runs: a file tool leaves them to git itself, which is a command.
	f.call("read", `{"path":".git/config"}`)
	f.refused("write", `{"path":".git/config","content":"[core]\n\tfsmonitor = ./payload.sh\n"}`, `\.git/config is in \.git`)
	f.refused("edit", `{"path":".git/config","oldString":"[core]","newString":"[core]\n\tfsmonitor = ./payload.sh"}`, `is in \.git`)
	f.refused("write", `{"path":".git/hooks/pre-commit","content":"#!/bin/sh\n"}`, `is in \.git`)

	// A repository that names a program for git to run, as one cloned from elsewhere may: listing its files runs
	// nothing it says.
	ran := filepath.Join(f.workspace, "RAN")
	f.put("payload.sh", "#!/bin/sh\ntouch "+ran+"\n")
	if err := os.Chmod(filepath.Join(f.workspace, "payload.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", f.workspace, "config", "core.fsmonitor", "./payload.sh").CombinedOutput(); err != nil {
		t.Fatalf("%s", output)
	}
	f.put("src/kept.ts", "const needle = 1;\n")
	f.outputs("glob", `{"pattern":"**/*.ts"}`, "src/kept.ts")
	f.call("grep", `{"pattern":"needle"}`)
	if _, err := os.Stat(ran); err == nil {
		t.Fatal("listing the files ran the program the repository named")
	}
}

func fakeWeb() *web.Port {
	page := web.Page{Text: "PAGE", Title: "Timeouts", Truncated: true, URL: "https://docs.test/timeouts"}
	return &web.Port{
		Read: func(_ context.Context, address, question string) web.ReadOutcome {
			if strings.HasSuffix(address, "/gone") {
				return web.ReadOutcome{Problem: address + " answered 404."}
			}
			return web.ReadOutcome{Answer: "Answer to: " + question, OK: true, Page: page}
		},
		Search: func(_ context.Context, query string) web.SearchOutcome {
			if query == "nothing" {
				return web.SearchOutcome{OK: true}
			}
			return web.SearchOutcome{OK: true, Results: []web.SearchResult{
				{Snippet: "Set requestTimeout.", Title: "Timeouts", URL: "https://docs.test/timeouts"},
				{Title: "Server options", URL: "https://docs.test/server"},
			}}
		},
	}
}

func TestWebsearchListsTheResultsAndSaysWhenThereAreNone(t *testing.T) {
	f := setUp(t)
	f.tc.Web = fakeWeb()
	got := f.outputs("websearch", `{"query":"fastify timeout"}`, "1. Timeouts\n   https://docs.test/timeouts\n   Set requestTimeout.\n\n2. Server options\n   https://docs.test/server")
	if got.Meta != "2 results" {
		t.Fatalf("meta %q", got.Meta)
	}
	if got := f.call("websearch", `{"query":"nothing"}`); got.Meta != "0 results" || !strings.Contains(got.Output, "No results") {
		t.Fatalf("got %+v", got)
	}
	f.tc.Web = nil
	f.refused("websearch", `{"query":"x"}`, `^The web cannot be reached from here.$`)
}

func TestWebfetchAnswersAQuestionAboutAPageWithItsSource(t *testing.T) {
	f := setUp(t)
	f.tc.Web = fakeWeb()
	got := f.outputs("webfetch", `{"question":"What is the default?","url":"https://docs.test/timeouts"}`, "Answer to: What is the default?\n\nSource: https://docs.test/timeouts (only the start of the page was read)")
	if got.Meta != "docs.test" {
		t.Fatalf("meta %q", got.Meta)
	}
	if got := f.call("webfetch", `{"question":"x","url":"https://docs.test/gone"}`); got.OK || got.Meta != "failed" || got.Output != "https://docs.test/gone answered 404." {
		t.Fatalf("got %+v", got)
	}
	f.refused("webfetch", `{"question":"x","url":"file:///etc/passwd"}`, `does not fit`)
}

var png = []byte{0x89, 0x50, 0x4e, 0x47, 9, 9}

type fakeImages struct{ asked []images.DrawRequest }

func (f *fakeImages) Draw(_ context.Context, request images.DrawRequest) images.DrawOutcome {
	f.asked = append(f.asked, request)
	return images.DrawOutcome{OK: true, PNG: png}
}

func (f *fakeImages) Keep([]byte) (string, error) { return "/api/images/s1/p1.png", nil }

// Kept knows the picture the chat drew before, p0.png.
func (f *fakeImages) Kept(name string) ([]byte, error) {
	if name != "p0.png" {
		return nil, os.ErrNotExist
	}
	return []byte("P0"), nil
}

func TestImageSavesThePictureInTheWorkspaceAndShowsTheDaemonsCopy(t *testing.T) {
	f := setUp(t)
	drawer := &fakeImages{}
	f.tc.Images = drawer
	got := f.call("image", `{"path":"assets/fox.png","prompt":"a fox","size":"768x1024"}`)
	image, _ := got.Display.Value.(protocol.ToolDisplayImage)
	if !got.OK || got.Meta != "768x1024" || image.URL != "/api/images/s1/p1.png" || !strings.HasPrefix(got.Output, "Created assets/fox.png: a 768x1024 picture") {
		t.Fatalf("got %+v", got)
	}
	if !bytes.Equal([]byte(f.read("assets/fox.png")), png) {
		t.Fatal("not saved")
	}
	if !reflect.DeepEqual(drawer.asked, []images.DrawRequest{{Prompt: "a fox", References: []images.Reference{}, Size: "768x1024"}}) {
		t.Fatalf("asked %+v", drawer.asked)
	}
}

func TestImageSendsThePicturesToChangeInOrderWithTheirTypes(t *testing.T) {
	f := setUp(t)
	drawer := &fakeImages{}
	f.tc.Images = drawer
	f.put("logo.png", "LOGO")
	f.put("photos/me.jpg", "ME")
	f.call("image", `{"path":"out.png","prompt":"Put picture 1 on picture 2","references":["logo.png","photos/me.jpg"]}`)
	want := []images.Reference{{Data: []byte("LOGO"), Name: "logo.png", Type: "image/png"}, {Data: []byte("ME"), Name: "me.jpg", Type: "image/jpeg"}}
	if !reflect.DeepEqual(drawer.asked[0].References, want) || drawer.asked[0].Size != "1024x1024" {
		t.Fatalf("asked %+v", drawer.asked)
	}
}

func TestImageRefusesWhatItCannotDoBeforeAskingTheServer(t *testing.T) {
	f := setUp(t)
	drawer := &fakeImages{}
	f.tc.Images = drawer
	f.put("notes.txt", "text")
	f.refused("image", `{"path":"fox.jpg","prompt":"a fox"}`, `\.png`)
	f.refused("image", `{"path":"a.png","prompt":"x","references":["missing.png"]}`, `not a file`)
	f.refused("image", `{"path":"a.png","prompt":"x","references":["notes.txt"]}`, `not a PNG, JPEG or WebP`)
	f.refused("image", `{"path":"a.png","prompt":"x","size":"1000x1000"}`, `steps of 64`)
	f.tc.Images = nil
	f.refused("image", `{"path":"a.png","prompt":"x"}`, `SWCODE_IMAGE_URL`)
	if len(drawer.asked) != 0 {
		t.Fatal("the server was asked")
	}
}

type hostJobs struct{ f *fixture }

func (j hostJobs) Start(ctx context.Context, command string) (string, error) {
	return j.f.tc.Host.StartJob(ctx, command, j.f.workspace)
}
func (j hostJobs) Read(ctx context.Context, id string, consume bool) (*hosts.JobState, error) {
	return j.f.tc.Host.ReadJob(ctx, id, consume)
}
func (j hostJobs) Stop(ctx context.Context, id string) (*hosts.JobState, error) {
	return j.f.tc.Host.StopJob(ctx, id)
}

func TestBackgroundCommandsStartWithBashReadWithOutputAndStopWithKill(t *testing.T) {
	f := setUp(t)
	f.tc.Jobs = hostJobs{f}
	got := f.call("bash", `{"background":true,"command":"echo listening on 5173; sleep 30"}`)
	if got.Meta != "running" || !got.OK || !strings.HasPrefix(got.Output, "listening on 5173\n[job 1 still running") {
		t.Fatalf("got %+v", got)
	}
	if got := f.call("output", `{"id":"1"}`); !strings.HasPrefix(got.Output, "(nothing new)\n[job 1 still running") {
		t.Fatalf("got %q", got.Output)
	}
	if got := f.call("kill", `{"id":"1"}`); got.Meta != "stopped" || !got.OK {
		t.Fatalf("got %+v", got)
	}
	if got := f.call("output", `{"id":"9"}`); got.OK || got.Output != "There is no job 9 in this session." {
		t.Fatalf("got %+v", got)
	}
}

func TestABackgroundCommandSaysHowItEndedWhenItEndsByItself(t *testing.T) {
	f := setUp(t)
	f.tc.Jobs = hostJobs{f}
	if got := f.outputs("bash", `{"background":true,"command":"echo built; exit 3"}`, "built\n[job 1 exit code 3]"); got.Meta != "exit code 3" || got.OK {
		t.Fatalf("got %+v", got)
	}
}

func TestBashDoesNotHoldWhenACommandLeavesAProcessBehind(t *testing.T) {
	f := setUp(t)
	started := time.Now()
	f.outputs("bash", `{"command":"sleep 20 & echo left behind"}`, "left behind")
	if time.Since(started) > 5*time.Second {
		t.Fatalf("held %v", time.Since(started))
	}
}

type fakeScreenshots struct {
	seen  bool
	drawn []browser.ShotRequest
	asked []string
}

func (s *fakeScreenshots) Seen() bool { return s.seen }
func (s *fakeScreenshots) Capture(_ context.Context, request browser.ShotRequest) browser.ShotOutcome {
	s.drawn = append(s.drawn, request)
	return browser.ShotOutcome{OK: true, Shot: browser.Shot{Errors: []string{"Uncaught TypeError: x is undefined"}, Height: 900, PNG: png, Title: "Docs", Width: 1440}}
}
func (s *fakeScreenshots) Keep(browser.Shot) (protocol.PictureRef, string, error) {
	return protocol.PictureRef{File: "p1.png", Height: 900, Type: protocol.PictureTypeImagePng, Width: 1440}, "/api/images/s1/p1.png", nil
}
func (s *fakeScreenshots) Ask(_ context.Context, _ protocol.PictureRef, question, _ string) (string, error) {
	s.asked = append(s.asked, question)
	return "The two buttons sit on one row.", nil
}

func TestAScreenshotAnswersAQuestionFromThePictureWithThePagesErrors(t *testing.T) {
	f := setUp(t)
	shots := &fakeScreenshots{seen: true}
	f.tc.Screenshots = shots
	got := f.call("screenshot", `{"question":"Do the buttons stay on one row?","size":"phone","url":"http://localhost:5173/"}`)
	if !got.OK || got.Meta != "1440 × 900" || got.Pictures != nil {
		t.Fatalf("got %+v", got)
	}
	if !strings.Contains(got.Output, "The two buttons sit on one row.") || !strings.Contains(got.Output, "- Uncaught TypeError: x is undefined") {
		t.Fatalf("output %q", got.Output)
	}
	if !reflect.DeepEqual(shots.asked, []string{"Do the buttons stay on one row?"}) || shots.drawn[0].Viewport != (browser.Viewport{Height: 844, Mobile: true, Width: 390}) {
		t.Fatalf("asked %v, drawn %+v", shots.asked, shots.drawn)
	}
}

func TestAScreenshotIsGivenToTheAgentThatAsksToSeeIt(t *testing.T) {
	f := setUp(t)
	shots := &fakeScreenshots{seen: true}
	f.tc.Screenshots = shots
	got := f.call("screenshot", `{"see":true,"url":"http://localhost:5173/"}`)
	if !strings.Contains(got.Output, "It follows as a picture.") || len(got.Pictures) != 1 || len(shots.asked) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestAScreenshotIsRefusedWithoutABrowserAModelThatSeesOrAQuestion(t *testing.T) {
	f := setUp(t)
	f.refused("screenshot", `{"see":true,"url":"http://localhost:5173/"}`, `No browser`)
	f.tc.Screenshots = &fakeScreenshots{}
	f.refused("screenshot", `{"see":true,"url":"http://localhost:5173/"}`, `without vision`)
	f.tc.Screenshots = &fakeScreenshots{seen: true}
	f.refused("screenshot", `{"url":"http://localhost:5173/"}`, `question`)
}

func TestApplyEdit(t *testing.T) {
	cases := []struct {
		text, old, new string
		all            bool
		want           EditResult
	}{
		{"a b a", "b", "c", false, EditResult{OK: true, Replaced: 1, Text: "a c a"}},
		{"a b a", "a", "c", true, EditResult{OK: true, Replaced: 2, Text: "c b c"}},
		{"line1\r\nline2\r\n", "line1\nline2", "one\ntwo", false, EditResult{OK: true, Replaced: 1, Text: "one\r\ntwo\r\n"}},
	}
	for _, each := range cases {
		if got := ApplyEdit(each.text, each.old, each.new, each.all); got != each.want {
			t.Errorf("%q: got %+v", each.text, got)
		}
	}
	for _, problem := range []EditResult{ApplyEdit("a a", "a", "b", false), ApplyEdit("a", "x", "y", false), ApplyEdit("a", "", "y", false), ApplyEdit("a", "a", "a", false)} {
		if problem.OK || problem.Problem == "" {
			t.Errorf("got %+v", problem)
		}
	}
}

// goFile is a file indented with tabs, as gofmt leaves it.
const goFile = "func greet(name string) string {\n\tif name == \"\" {\n\t\treturn \"Hello\"\n\t}\n\treturn \"Hello, \" + name\n}\n"

func TestAnEditFindsItsTextByItsWordsWhenTheIndentationDiffersAndKeepsTheFiles(t *testing.T) {
	// The model wrote the block with four spaces, from memory: the file has tabs, and keeps them.
	wrote := "    if name == \"\" {\n        return \"Hello\"\n    }"
	with := "    if name == \"\" {\n        return \"Hi\"\n    }\n    name = strings.TrimSpace(name)"
	edited := ApplyEdit(goFile, wrote, with, false)
	want := "func greet(name string) string {\n\tif name == \"\" {\n\t\treturn \"Hi\"\n\t}\n\tname = strings.TrimSpace(name)\n\treturn \"Hello, \" + name\n}\n"
	if !edited.OK || edited.Replaced != 1 || edited.Text != want || edited.Note == "" {
		t.Fatalf("got %+v", edited)
	}
	// Text that is in the file as written is replaced as written, and says nothing of how it was found.
	if exact := ApplyEdit(goFile, "\treturn \"Hello, \" + name", "\treturn \"Hi, \" + name", false); !exact.OK || exact.Note != "" {
		t.Fatalf("got %+v", exact)
	}
}

func TestAnEditByItsWordsStillTakesOnePlaceUnlessAllAreAsked(t *testing.T) {
	twice := "a:\n\tx = 1\n\ty = 2\nb:\n\t\tx = 1\n\t\ty = 2\n"
	if refused := ApplyEdit(twice, "  x = 1\n  y = 2", "  x = 3", false); refused.OK || !strings.Contains(refused.Problem, "appears 2 times, indentation aside") {
		t.Fatalf("got %+v", refused)
	}
	all := ApplyEdit(twice, "  x = 1\n  y = 2", "  x = 3", true)
	if !all.OK || all.Replaced != 2 || all.Text != "a:\n\tx = 3\nb:\n\t\tx = 3\n" {
		t.Fatalf("got %+v", all)
	}
}

func TestTextThatIsNotInTheFileSaysWhereItsFirstLineIs(t *testing.T) {
	refused := ApplyEdit(goFile, "\tif name == \"\" {\n\t\treturn \"Bonjour\"", "x", false)
	if refused.OK || !strings.Contains(refused.Problem, "Its first line is at line 2 of the file") {
		t.Fatalf("got %+v", refused)
	}
	if nowhere := ApplyEdit(goFile, "nothing like it", "x", false); nowhere.OK || nowhere.Problem != notFound {
		t.Fatalf("got %+v", nowhere)
	}
}

func TestFileDiffNumbersTheLinesAroundEachChangeAGapBetween(t *testing.T) {
	before := strings.Join([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"}, "\n") + "\n"
	after := strings.Replace(strings.Replace(before, "2\n", "two\n", 1), "11\n", "eleven\n", 1)
	diff := FileDiff("a.txt", before, after)
	kinds := []string{}
	for _, line := range diff.Lines {
		kinds = append(kinds, string(line.Kind)[:1])
	}
	if strings.Join(kinds, "") != "crac"+"cc"+"g"+"ccc"+"ra"+"c" || diff.Added != 2 || diff.Removed != 2 {
		t.Fatalf("kinds %s, %+v", strings.Join(kinds, ""), diff)
	}
	if first := diff.Lines[1]; first.OldLine != 2 || first.Text != "2" {
		t.Fatalf("first change %+v", first)
	}
	created := FileDiff("new.txt", "", "a\nb\n")
	if created.Added != 2 || created.Lines[0].NewLine != 1 {
		t.Fatalf("created %+v", created)
	}
}

func TestTheToolSchemasAreWhatTheModelReads(t *testing.T) {
	data, _ := json.Marshal(readTool.Input)
	want := `{"type":"object","properties":{"limit":{"type":"number","minimum":1,"description":"How many lines to read; up to 2000 when left out."},"offset":{"type":"number","minimum":1,"description":"The line to start from, counting from 1. For long files."},"path":{"type":"string","minLength":1,"description":"The file, relative to the workspace."}},"required":["path"],"additionalProperties":false}`
	if string(data) != want {
		t.Fatalf("got %s", data)
	}
	main, explore, chat := Boxes(Servers{})
	names := func(box Box) []string {
		list := []string{}
		for _, definition := range box.Definitions() {
			list = append(list, definition.Name)
		}
		return list
	}
	if got := names(explore); !reflect.DeepEqual(got, []string{"read", "grep", "glob", "websearch", "webfetch"}) {
		t.Fatalf("explore %v", got)
	}
	if got := names(main); strings.Contains(strings.Join(got, " "), "image") || strings.Contains(strings.Join(got, " "), "screenshot") {
		t.Fatalf("main %v", got)
	}
	if got := names(chat); !reflect.DeepEqual(got, []string{"websearch", "webfetch"}) {
		t.Fatalf("chat %v", got)
	}
	main, _, chat = Boxes(Servers{Images: true})
	if got := names(chat); !reflect.DeepEqual(got, []string{"websearch", "webfetch", "draw"}) {
		t.Fatalf("chat with pictures %v", got)
	}
	if got := names(main); slices.Contains(got, "draw") || !slices.Contains(got, "image") {
		t.Fatalf("main with pictures %v", got)
	}
}

func TestDrawShowsThePictureInTheChatAndChangesOneItDrewByItsName(t *testing.T) {
	f := setUp(t)
	drawer := &fakeImages{}
	f.tc.Images = drawer
	got := f.call("draw", `{"prompt":"the fox, in blue","references":["p0.png"],"size":"768x1024"}`)
	image, _ := got.Display.Value.(protocol.ToolDisplayImage)
	if !got.OK || got.Meta != "768x1024" || image.URL != "/api/images/s1/p1.png" || image.Path != "p1.png" || !strings.HasPrefix(got.Output, "Drew p1.png: a 768x1024 picture") {
		t.Fatalf("got %+v", got)
	}
	want := []images.DrawRequest{{Prompt: "the fox, in blue", References: []images.Reference{{Data: []byte("P0"), Name: "p0.png", Type: "image/png"}}, Size: "768x1024"}}
	if !reflect.DeepEqual(drawer.asked, want) {
		t.Fatalf("asked %+v", drawer.asked)
	}
}

func TestDrawRefusesAPictureItDidNotDrawBeforeAskingTheServer(t *testing.T) {
	f := setUp(t)
	drawer := &fakeImages{}
	f.tc.Images = drawer
	f.refused("draw", `{"prompt":"x","references":["../secret.png"]}`, `not a picture of this chat`)
	f.refused("draw", `{"prompt":"x","size":"1000x1000"}`, `steps of 64`)
	f.tc.Images = nil
	f.refused("draw", `{"prompt":"x"}`, `SWCODE_IMAGE_URL`)
	if len(drawer.asked) != 0 {
		t.Fatal("the server was asked")
	}
}
