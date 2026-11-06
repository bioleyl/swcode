package tools

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"swcode/daemon/browser"
	"swcode/daemon/hosts"
	"swcode/daemon/images"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
)

const (
	defaultPictureSize = "1024x1024"
	minSide            = 256
	maxSide            = 1536
	// sideStep: the model draws in blocks of 64 pixels.
	sideStep      = 64
	maxReferences = 4
	bytesPerKB    = 1024
	pngExtension  = ".png"
	noServer      = "No picture server is set up (SWCODE_IMAGE_URL)."
	notPNG        = "The picture is saved as a PNG: give a path ending in .png."
	shotTimeout   = 30 * time.Second
	noBrowser     = "No browser to draw pages with here."
	unseen        = "The main model is loaded without vision: no model here can look at a screenshot (llm-server needs its vision projector, mmproj)."
	askOrSee      = "Give a question about the page, or see: true to look at it yourself."
	desktopSize   = "desktop"
	phoneSize     = "phone"
)

var (
	pictureSize  = regexp.MustCompile(`^(\d+)x(\d+)$`)
	pictureTypes = map[string]string{".jpeg": "image/jpeg", ".jpg": "image/jpeg", ".png": "image/png", ".webp": "image/webp"}
	viewports    = map[string]browser.Viewport{
		desktopSize: {Height: 900, Width: 1440},
		phoneSize:   {Height: 844, Mobile: true, Width: 390},
	}
)

func fitsServer(size string) bool {
	found := pictureSize.FindStringSubmatch(size)
	if found == nil {
		return false
	}
	for _, text := range found[1:] {
		side, _ := strconv.Atoi(text)
		if side < minSide || side > maxSide || side%sideStep != 0 {
			return false
		}
	}
	return true
}

func extensionOf(paths hosts.Paths, path string) string {
	base := paths.Base(path)
	dot := strings.LastIndex(base, ".")
	if dot <= 0 {
		return ""
	}
	return strings.ToLower(base[dot:])
}

type imageInput struct {
	Path       string   `json:"path"`
	Prompt     string   `json:"prompt"`
	References []string `json:"references"`
	Size       string   `json:"size"`
}

type reference struct {
	path        string
	contentType string
}

// checkReference: a reference must be a picture file of the workspace, of a type the picture server reads.
func checkReference(ctx context.Context, tc *Context, input string) (reference, string) {
	path, problem := resolveTarget(ctx, tc, input)
	if problem != "" {
		return reference{}, problem
	}
	contentType, known := pictureTypes[extensionOf(tc.Host.Paths(), path)]
	if !known {
		return reference{}, input + " is not a PNG, JPEG or WebP picture."
	}
	if info, err := tc.Host.FileInfo(ctx, path); err != nil || info.Kind != hosts.KindFile {
		return reference{}, input + " is not a file."
	}
	return reference{contentType: contentType, path: path}, ""
}

func drawPicture(ctx context.Context, tc *Context, input imageInput, path string, references []reference, existed bool) Outcome {
	pictures := []images.Reference{}
	for _, each := range references {
		data, err := tc.Host.ReadFile(ctx, each.path)
		if err != nil {
			return Failure(err.Error(), "failed")
		}
		pictures = append(pictures, images.Reference{Data: data, Name: tc.Host.Paths().Base(each.path), Type: each.contentType})
	}
	size := input.Size
	if size == "" {
		size = defaultPictureSize
	}
	picture := tc.Images.Draw(ctx, images.DrawRequest{Prompt: input.Prompt, References: pictures, Size: size})
	if !picture.OK {
		return Failure(picture.Problem, "failed")
	}
	if err := tc.Host.WriteFile(ctx, path, picture.PNG); err != nil {
		return Failure(err.Error(), "failed")
	}
	address, err := tc.Images.Keep(picture.PNG)
	if err != nil {
		return Failure(err.Error(), "failed")
	}
	label := WorkspacePath(tc.Host.Paths(), tc.Workspace, path)
	verb := "Created"
	if existed {
		verb = "Replaced"
	}
	return Outcome{
		Display: &protocol.ToolDisplay{Value: protocol.ToolDisplayImage{Path: label, URL: address}},
		Meta:    size,
		OK:      true,
		Output:  fmt.Sprintf("%s %s: a %s picture (%d kB). The user sees it.", verb, label, size, int(math.Round(float64(len(picture.PNG))/bytesPerKB))),
	}
}

// imageTool makes a picture with the picture server of the settings and saves it in the workspace, as a PNG: from
// a prompt, or from pictures of the workspace and a prompt. The daemon keeps a copy for the app to show.
var imageTool = Define(Spec[imageInput]{
	Check: func(ctx context.Context, input imageInput, tc *Context) Check {
		if tc.Images == nil {
			return Refuse(noServer)
		}
		if extensionOf(tc.Host.Paths(), input.Path) != pngExtension {
			return Refuse(notPNG)
		}
		if input.Size != "" && !fitsServer(input.Size) {
			return Refuse(fmt.Sprintf("size: each side from %d to %d pixels, in steps of %d", minSide, maxSide, sideStep))
		}
		path, problem := resolveTarget(ctx, tc, input.Path)
		if problem != "" {
			return Refuse(problem)
		}
		info, err := tc.Host.FileInfo(ctx, path)
		if err != nil {
			return Refuse(err.Error())
		}
		if info.Kind == hosts.KindDirectory {
			return Refuse(input.Path + " is a folder.")
		}
		references := []reference{}
		for _, each := range input.References {
			checked, problem := checkReference(ctx, tc, each)
			if problem != "" {
				return Refuse(problem)
			}
			references = append(references, checked)
		}
		return Ready(func(ctx context.Context) Outcome {
			return drawPicture(ctx, tc, input, path, references, info.Kind == hosts.KindFile)
		}, nil)
	},
	Description: "Makes a picture (an illustration, an icon, a mock-up, a photo) with the picture server and saves it as a PNG in the workspace. With references, it changes pictures of the workspace or draws from them instead. Takes about a minute. The user sees the picture; you do not.",
	Input: Object(
		Required("path", filePath.Describe("Where to save the picture, relative to the workspace: a .png file.")),
		Required("prompt", String().Trimmed().Min(1).Describe(`The picture in full: subject, composition, style, colours, light, and any text it shows, word for word. With references, what to change or draw from them, naming them by number ("picture 1 is the logo").`)),
		Optional("references", Array(filePath).Max(maxReferences).Describe("Pictures of the workspace to change or draw from (PNG, JPEG, WebP), in the order the prompt numbers them.")),
		Optional("size", String().Matching(`^(\d+)x(\d+)$`).Describe(fmt.Sprintf("WIDTHxHEIGHT in pixels: %s when left out; 768x1024 for a portrait, 1280x768 for a wide one.", defaultPictureSize))),
	),
	Name:      protocol.ToolNameImage,
	Subject:   func(input imageInput, _ *Context) permissions.Subject { return edits(input.Path) },
	Summarize: func(input imageInput) string { return input.Path },
})

// Screenshots is screenshots for a turn's tools: drawn by the browser, kept with the session's pictures, read by a
// model that sees.
type Screenshots interface {
	// Seen tells whether the main model sees pictures: the tool is of no use otherwise.
	Seen() bool
	Capture(ctx context.Context, request browser.ShotRequest) browser.ShotOutcome
	// Keep keeps a screenshot among the session's pictures: how the model gets it, and the address the app shows
	// it at.
	Keep(shot browser.Shot) (protocol.PictureRef, string, error)
	// Ask has a question about a screenshot answered in a request of its own, so the picture stays out of the
	// agent's context.
	Ask(ctx context.Context, picture protocol.PictureRef, question, about string) (string, error)
}

type screenshotInput struct {
	FullPage bool   `json:"fullPage"`
	Question string `json:"question"`
	See      bool   `json:"see"`
	Size     string `json:"size"`
	URL      string `json:"url"`
	WaitFor  string `json:"waitFor"`
}

func (input screenshotInput) size() string {
	if input.Size == "" {
		return desktopSize
	}
	return input.Size
}

// aboutOf is what the page is, for the model that looks at it and for the agent.
func aboutOf(input screenshotInput, shot browser.Shot) string {
	title := ""
	if shot.Title != "" {
		title = fmt.Sprintf(` ("%s")`, shot.Title)
	}
	return fmt.Sprintf("Screenshot of %s%s, %s, %d × %d.", input.URL, title, input.size(), shot.Width, shot.Height)
}

func errorsOf(shot browser.Shot) string {
	if len(shot.Errors) == 0 {
		return "The page logged no error."
	}
	lines := []string{"The page logged errors:"}
	for _, each := range shot.Errors {
		lines = append(lines, "- "+each)
	}
	return strings.Join(lines, "\n")
}

func takeScreenshot(ctx context.Context, shots Screenshots, input screenshotInput) Outcome {
	drawn := shots.Capture(ctx, browser.ShotRequest{
		FullPage: input.FullPage, Timeout: shotTimeout, URL: input.URL, Viewport: viewports[input.size()], WaitFor: input.WaitFor,
	})
	if !drawn.OK {
		return Failure(drawn.Problem, "failed")
	}
	picture, address, err := shots.Keep(drawn.Shot)
	if err != nil {
		return Failure(err.Error(), "failed")
	}
	about := aboutOf(input, drawn.Shot)
	answer := ""
	if input.Question != "" {
		if answer, err = shots.Ask(ctx, picture, input.Question, about); err != nil {
			return Failure(err.Error(), "failed")
		}
	}
	seeing := ""
	if input.See {
		seeing = "It follows as a picture."
	}
	outcome := Outcome{
		Display: &protocol.ToolDisplay{Value: protocol.ToolDisplayImage{Path: input.URL, URL: address}},
		Meta:    fmt.Sprintf("%d × %d", drawn.Shot.Width, drawn.Shot.Height),
		OK:      true,
		Output:  strings.Join(nonEmpty(about, answer, seeing, errorsOf(drawn.Shot)), "\n\n"),
	}
	if input.See {
		outcome.Pictures = []protocol.PictureRef{picture}
	}
	return outcome
}

// screenshotTool draws a web page in a browser and looks at it: a model that sees answers the agent's question
// from the picture in a request of its own, or the picture comes into the agent's own context when it asks to see
// it. The person sees it in the conversation either way.
var screenshotTool = Define(Spec[screenshotInput]{
	Check: func(_ context.Context, input screenshotInput, tc *Context) Check {
		switch {
		case input.Question == "" && !input.See:
			return Refuse(askOrSee)
		case tc.Screenshots == nil:
			return Refuse(noBrowser)
		case !tc.Screenshots.Seen():
			return Refuse(unseen)
		}
		return Ready(func(ctx context.Context) Outcome { return takeScreenshot(ctx, tc.Screenshots, input) }, nil)
	},
	Description: "Draws a web page in a browser, at a desktop or a phone size, and answers a question about how it looks; set see to look at it yourself. The user sees it too. Start a dev server first, with bash in the background.",
	Input: Object(
		Optional("fullPage", Boolean().Describe("The whole page, scrolled, rather than the window (4000 px at most).")),
		Optional("question", String().Trimmed().Min(1).Describe(`What you need to know from the picture: "Do the send and stop buttons stay on one row?". A model that sees answers it from the screenshot, which stays out of your context.`)),
		Optional("see", Boolean().Describe("Put the screenshot itself in your context, to look at it yourself, when a question is not enough: it costs a thousand tokens or so.")),
		Optional("size", Enum(desktopSize, phoneSize).Describe("desktop: 1440 × 900, the default; phone: 390 × 844, as a phone.")),
		Required("url", String().WebAddress().Describe("The page: a dev server running in the background (http://localhost:5173/), or any web page.")),
		Optional("waitFor", String().Trimmed().Min(1).Describe("Text the page shows once ready: the picture waits for it.")),
	),
	Name:      protocol.ToolNameScreenshot,
	Subject:   reads[screenshotInput],
	Summarize: func(input screenshotInput) string { return input.URL },
})
