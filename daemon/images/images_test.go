package images

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

const server = "http://pictures.test:8090"

var png = []byte{0x89, 0x50, 0x4e, 0x47, 1, 2, 3}

type asked struct {
	url  string
	body any
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

// fakeServer is a picture server keeping what it was asked: the address, and the body as JSON or as a form.
func fakeServer(status int, body string) (*http.Client, *[]asked) {
	requests := &[]asked{}
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		*requests = append(*requests, asked{body: bodyOf(request), url: request.URL.String()})
		return &http.Response{Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, StatusCode: status}, nil
	})}, requests
}

func bodyOf(request *http.Request) any {
	data, _ := io.ReadAll(request.Body)
	kind, params, _ := mime.ParseMediaType(request.Header.Get("content-type"))
	if kind != "multipart/form-data" {
		var object map[string]any
		_ = json.Unmarshal(data, &object)
		return object
	}
	fields := [][2]string{}
	reader := multipart.NewReader(bytes.NewReader(data), params["boundary"])
	for part, err := reader.NextPart(); err == nil; part, err = reader.NextPart() {
		value, _ := io.ReadAll(part)
		fields = append(fields, [2]string{part.FormName(), string(value)})
	}
	return fields
}

func drawn() string {
	answer, _ := json.Marshal(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(png)}}})
	return string(answer)
}

func TestAPictureFromAPromptAloneIsAskedAsJSON(t *testing.T) {
	client, requests := fakeServer(http.StatusOK, drawn())
	got := DrawPicture(context.Background(), server, DrawRequest{Prompt: "a fox", Size: "768x1024"}, client)
	if !got.OK || !bytes.Equal(got.PNG, png) {
		t.Fatalf("got %+v", got)
	}
	want := []asked{{body: map[string]any{"n": float64(1), "prompt": "a fox", "size": "768x1024"}, url: server + "/v1/images/generations"}}
	if !reflect.DeepEqual(*requests, want) {
		t.Fatalf("asked %+v", *requests)
	}
}

func TestPicturesToChangeGoInOrderAsAFormToTheEditsRoute(t *testing.T) {
	client, requests := fakeServer(http.StatusOK, drawn())
	DrawPicture(context.Background(), server, DrawRequest{Prompt: "Put picture 1 on picture 2", Size: "1024x1024", References: []Reference{
		{Data: []byte("logo"), Name: "logo.png", Type: "image/png"},
		{Data: []byte("photo"), Name: "me.jpg", Type: "image/jpeg"},
	}}, client)
	want := []asked{{body: [][2]string{
		{"prompt", "Put picture 1 on picture 2"}, {"n", "1"}, {"size", "1024x1024"}, {"image[]", "logo"}, {"image[]", "photo"},
	}, url: server + "/v1/images/edits"}}
	if !reflect.DeepEqual(*requests, want) {
		t.Fatalf("asked %+v", *requests)
	}
}

func TestThePictureServerSaysWhatWentWrong(t *testing.T) {
	busy, _ := fakeServer(http.StatusInternalServerError, "out of memory")
	if got := DrawPicture(context.Background(), server, DrawRequest{Prompt: "x", Size: "512x512"}, busy); got.Problem != "The picture server answered 500: out of memory" {
		t.Fatalf("got %+v", got)
	}
	down := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connect ECONNREFUSED")
	})}
	if got := DrawPicture(context.Background(), server, DrawRequest{Prompt: "x", Size: "512x512"}, down); !strings.Contains(got.Problem, "ECONNREFUSED") {
		t.Fatalf("got %+v", got)
	}
}
