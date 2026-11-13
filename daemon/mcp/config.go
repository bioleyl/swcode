// Package mcp is swcode's client of the Model Context Protocol: it starts the servers the person lists (a program
// talked to over its standard streams, or an address talked to over HTTP), asks each for its tools, and calls them
// for the agent. Tools only: no resources, no prompts, no sampling.
package mcp

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

// File is the servers file, in the data folder. Its shape is the one other agents' files have, so a server's
// entry can be copied from its documentation as it is.
const File = "mcp.json"

// ServerConfig is one server of the file: a program and its arguments, or an address.
type ServerConfig struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// servers is the file's content: the servers, by the name the person gave each.
type servers struct {
	Servers map[string]ServerConfig `json:"mcpServers"`
}

// Load reads the servers file: none when there is no file, an error when it is not what the file should be.
func Load(path string) (map[string]ServerConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]ServerConfig{}, nil
	}
	if err != nil {
		return nil, err
	}
	var file servers
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, errors.New(File + " is not valid JSON: " + err.Error())
	}
	if file.Servers == nil {
		return map[string]ServerConfig{}, nil
	}
	return file.Servers, nil
}
