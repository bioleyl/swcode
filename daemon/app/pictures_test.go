package app_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"swcode/daemon/agent"
	"swcode/daemon/browser"
	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

// pngData is a 1 × 1 PNG.
const pngData = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

var screenshot = protocol.PictureUpload{Data: pngData, Height: 720, Type: protocol.PictureTypeImagePng, Width: 1280}

func userMessages(test *App, request int) []map[string]any {
	messages := []map[string]any{}
	for _, message := range fakellama.Messages(test.Requests()[request]) {
		if message["role"] == "user" {
			messages = append(messages, message)
		}
	}
	return messages
}

func picturesOf(test *App, sessionID string, index int) []protocol.PictureRef {
	seen := 0
	for _, event := range test.Events(sessionID) {
		if message, ok := event.Value.(protocol.SessionEventUserMessage); ok {
			if seen == index {
				return message.Pictures
			}
			seen++
		}
	}
	return nil
}

func TestAPastedPictureIsKeptAndAModelThatSeesGetsItAfterTheText(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("A red dot.")}, Vision: true})
	session := test.NewSession(sessions.Request{Pictures: []protocol.PictureUpload{screenshot}, Prompt: "What is this?"})
	test.UntilIdle(session.ID)
	picture := picturesOf(test, session.ID, 0)[0]
	if !regexp.MustCompile(`^[\w-]+\.png$`).MatchString(picture.File) || picture.Height != 720 || picture.Width != 1280 {
		t.Fatalf("picture %+v", picture)
	}
	data, _ := os.ReadFile(filepath.Join(test.DataDir, "images", session.ID, picture.File))
	if base64.StdEncoding.EncodeToString(data) != pngData {
		t.Fatal("not kept")
	}
	content := userMessages(test, 0)[0]["content"].([]any)
	first, second := content[0].(map[string]any), content[1].(map[string]any)
	if !strings.HasSuffix(first["text"].(string), "\nWhat is this?") || second["image_url"].(map[string]any)["url"] != "data:image/png;base64,"+pngData {
		t.Fatalf("content %v", content)
	}
	request, _ := http.NewRequest("GET", test.Server.URL+protocol.EndpointImages+"/"+session.ID+"/"+picture.File, nil)
	request.Header.Set("authorization", protocol.BearerPrefix+Token)
	served, err := http.DefaultClient.Do(request)
	if err != nil || served.StatusCode != 200 || served.Header.Get("content-type") != "image/png" {
		t.Fatalf("served %v %v", served, err)
	}
	served.Body.Close()
}

func TestAPastedPictureIsOnlyNamedToAModelThatDoesNotSee(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("I cannot see it.")}})
	session := test.NewSession(sessions.Request{Pictures: []protocol.PictureUpload{screenshot}, Prompt: "What is this?"})
	test.UntilIdle(session.ID)
	if content, _ := userMessages(test, 0)[0]["content"].(string); !strings.Contains(content, "What is this?\n\n[The user pasted a picture here, which you cannot see") {
		t.Fatalf("content %q", content)
	}
}

func TestAPictureMayComeAloneAndWaitsWithItsMessageWhileATurnRuns(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{{Delay: 300 * time.Millisecond, Text: "First."}, text("Second.")}, Vision: true})
	session := test.NewSession(sessions.Request{Prompt: "Start"})
	test.WaitForEvent("turnStarted", 1)
	if problem := test.App.Sessions.SendPrompt(session.ID, "", []protocol.PictureUpload{screenshot}); problem != "" {
		t.Fatal(problem)
	}
	Expect(t, test.App.Sessions.List()[0], `{"queued":["[1 picture]"]}`)
	test.WaitForEvent("turnFinished", 2)
	if len(picturesOf(test, session.ID, 1)) != 1 || !strings.Contains(RequestText(map[string]any{"m": userMessages(test, 1)}), "data:image/png;base64") {
		t.Fatal("the picture did not go with its message")
	}
}

func TestAPictureIsNeededWhenThereAreNoWords(t *testing.T) {
	test := Start(t, Options{})
	session := test.NewSession(sessions.Request{})
	if problem := test.App.Sessions.SendPrompt(session.ID, "", nil); problem != "Write a message or paste a picture first." {
		t.Fatalf("got %q", problem)
	}
	if _, err := os.Stat(filepath.Join(test.DataDir, "images", session.ID)); err == nil {
		t.Fatal("a pictures folder was made")
	}
}

func TestPicturesInTheContext(t *testing.T) {
	withPicture := protocol.SessionEventUserMessage{At: "2026-10-25T12:00:00.000Z", ID: "u1", Pictures: []protocol.PictureRef{{File: "a.png", Height: 720, Type: protocol.PictureTypeImagePng, Width: 1280}}}
	if got := agent.EstimatedTokens([]protocol.SessionEvent{{Value: withPicture}}, 0); got != 40*23 {
		t.Fatalf("tokens %d", got)
	}
	messages := agent.ConversationOf([]protocol.SessionEvent{{Value: withPicture}, {Value: protocol.SessionEventSkillsListed{ID: "s1", Skills: []protocol.SkillSummary{}}}}, &agent.PictureView{Folder: "/data/images/s", Seen: true})
	if !strings.Contains(messages[0].Content, "The user has no skills now.") || messages[0].Pictures[0].Path != filepath.Join("/data/images/s", "a.png") {
		t.Fatalf("messages %+v", messages)
	}
	withPicture.Text = "Look"
	if got := agent.TranscriptOf([]protocol.SessionEvent{{Value: withPicture}}, 1000); got != "User: Look [1 pasted pictures]" {
		t.Fatalf("transcript %q", got)
	}
}

type fakeBrowser struct{}

func (fakeBrowser) Capture(context.Context, browser.ShotRequest) browser.ShotOutcome {
	data, _ := base64.StdEncoding.DecodeString(pngData)
	return browser.ShotOutcome{OK: true, Shot: browser.Shot{Height: 1, PNG: data, Title: "Docs", Width: 1}}
}

func screenshotCall(input map[string]any) fakellama.Reply {
	input["url"] = "http://localhost:5173/"
	return call("screenshot", input)
}

func TestAScreenshotIsReadInARequestOfItsOwn(t *testing.T) {
	test := Start(t, Options{
		Browser: fakeBrowser{},
		Replies: []fakellama.Reply{screenshotCall(map[string]any{"question": "Is the title there?"}), text("It looks right.")},
		Scripts: map[string][]fakellama.Reply{"Question: Is the title there?": {text(`Yes, "Docs" at the top.`)}},
		Vision:  true,
	})
	session := test.NewSession(sessions.Request{Prompt: "Check the docs page"})
	test.UntilIdle(session.ID)
	finished := Finished(test.Events(session.ID))[0]
	if !finished.OK || !strings.Contains(finished.Output, `Yes, "Docs" at the top.`) {
		t.Fatalf("finished %+v", finished)
	}
	looked := false
	for _, request := range test.Requests() {
		if text := RequestText(request); strings.Contains(text, "You look at a screenshot") {
			looked = strings.Contains(text, "data:image/png;base64,"+pngData)
		}
	}
	requests := test.Requests()
	if !looked || strings.Contains(RequestText(requests[len(requests)-1]), "data:image/png") {
		t.Fatal("the picture went where it should not")
	}
}

func TestAScreenshotComesIntoTheAgentsContextWhenItAsksToSeeIt(t *testing.T) {
	test := Start(t, Options{Browser: fakeBrowser{}, Replies: []fakellama.Reply{screenshotCall(map[string]any{"see": true}), text("I see it.")}, Vision: true})
	session := test.NewSession(sessions.Request{Prompt: "Look at the docs page"})
	test.UntilIdle(session.ID)
	requests := test.Requests()
	messages := fakellama.Messages(requests[len(requests)-1])
	if messages[len(messages)-2]["role"] != "tool" {
		t.Fatalf("messages %v", messages)
	}
	Expect(t, messages[len(messages)-1]["content"], []any{
		map[string]any{"text": "[The screenshots you asked to see, in the order of your calls.]", "type": "text"},
		map[string]any{"image_url": map[string]any{"url": "data:image/png;base64," + pngData}, "type": "image_url"},
	})
}
