// Package images is the pictures of a session: those the picture server draws for its agent, and those the person
// pastes, kept in the daemon's folder for the app to show.
package images

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"swcode/daemon/protocol"
)

const (
	pngExtension = ".png"
	// sd-server's OpenAI-compatible routes: a picture from a prompt, or from pictures and a prompt.
	generationsPath = "/v1/images/generations"
	editsPath       = "/v1/images/edits"
	referenceField  = "image[]"
	// A picture takes about a minute, several references a few: well under this.
	drawTimeout     = 10 * time.Minute
	maxProblemChars = 200
)

var extensions = map[protocol.PictureType]string{protocol.PictureTypeImageJpeg: ".jpg", protocol.PictureTypeImagePng: ".png"}

// Reference is a picture to change or draw from, as the picture server takes it.
type Reference struct {
	Name string
	// Type is its media type: image/png, image/jpeg, image/webp.
	Type string
	Data []byte
}

// DrawRequest is a picture to draw: its prompt, its size as WIDTHxHEIGHT, and the pictures it starts from.
type DrawRequest struct {
	Prompt     string
	Size       string
	References []Reference
}

// DrawOutcome is a PNG, or why there is none.
type DrawOutcome struct {
	OK      bool
	PNG     []byte
	Problem string
}

// Port is pictures for a session's tools: drawn by the picture server, and a copy kept by the daemon for the app.
type Port interface {
	Draw(ctx context.Context, request DrawRequest) DrawOutcome
	// Keep keeps a copy of a picture, and gives the address the app shows it from.
	Keep(png []byte) (string, error)
}

type port struct {
	serverURL string
	folder    string
	client    *http.Client
	sessionID string
	newID     func() string
}

// NewPort is a session's pictures: drawn by the picture server, a copy kept in the daemon's folder for the app.
func NewPort(serverURL, folder string, client *http.Client, sessionID string, newID func() string) Port {
	return port{client: client, folder: folder, newID: newID, serverURL: serverURL, sessionID: sessionID}
}

func (p port) Draw(ctx context.Context, request DrawRequest) DrawOutcome {
	return DrawPicture(ctx, p.serverURL, request, p.client)
}

func (p port) Keep(png []byte) (string, error) {
	name := p.newID() + pngExtension
	folder := filepath.Join(p.folder, p.sessionID)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(folder, name), png, 0o644); err != nil {
		return "", err
	}
	return protocol.EndpointImages + "/" + p.sessionID + "/" + name, nil
}

// Remove forgets a session's pictures, with the session.
func Remove(folder, sessionID string) error {
	return os.RemoveAll(filepath.Join(folder, sessionID))
}

// KeepPictures keeps the pictures a person pasted in the session's pictures folder, beside those its agent made: the
// session's events only name them, so its file stays small. They go with the session.
func KeepPictures(folder string, uploads []protocol.PictureUpload, newID func() string) ([]protocol.PictureRef, error) {
	if len(uploads) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return nil, err
	}
	kept := []protocol.PictureRef{}
	for _, upload := range uploads {
		file := newID() + extensions[upload.Type]
		data, err := base64.StdEncoding.DecodeString(upload.Data)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(folder, file), data, 0o644); err != nil {
			return nil, err
		}
		kept = append(kept, protocol.PictureRef{File: file, Height: upload.Height, Type: upload.Type, Width: upload.Width})
	}
	return kept, nil
}

// requestOf: a prompt alone goes as JSON; with pictures, as a form, the pictures in order.
func requestOf(ctx context.Context, serverURL string, draw DrawRequest) (*http.Request, error) {
	base := strings.TrimRight(serverURL, "/")
	if len(draw.References) == 0 {
		body, _ := json.Marshal(map[string]any{"n": 1, "prompt": draw.Prompt, "size": draw.Size})
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+generationsPath, bytes.NewReader(body))
		if err == nil {
			request.Header.Set("content-type", "application/json")
		}
		return request, err
	}
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	_ = writer.WriteField("prompt", draw.Prompt)
	_ = writer.WriteField("n", "1")
	_ = writer.WriteField("size", draw.Size)
	for _, picture := range draw.References {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, referenceField, picture.Name))
		header.Set("Content-Type", picture.Type)
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, err
		}
		_, _ = part.Write(picture.Data)
	}
	_ = writer.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+editsPath, &form)
	if err == nil {
		request.Header.Set("content-type", writer.FormDataContentType())
	}
	return request, err
}

// DrawPicture asks a stable-diffusion.cpp sd-server for one picture, as a PNG.
func DrawPicture(ctx context.Context, serverURL string, draw DrawRequest, client *http.Client) DrawOutcome {
	ctx, cancel := context.WithTimeout(ctx, drawTimeout)
	defer cancel()
	request, err := requestOf(ctx, serverURL, draw)
	if err != nil {
		return DrawOutcome{Problem: "The picture server could not be asked: " + err.Error()}
	}
	response, err := client.Do(request)
	if err != nil {
		return DrawOutcome{Problem: "The picture server could not be asked: " + err.Error()}
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode >= http.StatusBadRequest {
		text := []rune(string(body))
		return DrawOutcome{Problem: fmt.Sprintf("The picture server answered %d: %s", response.StatusCode, string(text[:min(len(text), maxProblemChars)]))}
	}
	var answer struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &answer); err != nil || len(answer.Data) == 0 || answer.Data[0].B64JSON == "" {
		return DrawOutcome{Problem: "The picture server could not be asked: its answer holds no picture."}
	}
	png, err := base64.StdEncoding.DecodeString(answer.Data[0].B64JSON)
	if err != nil {
		return DrawOutcome{Problem: "The picture server could not be asked: " + err.Error()}
	}
	return DrawOutcome{OK: true, PNG: png}
}
