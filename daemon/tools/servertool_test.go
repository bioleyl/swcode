package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
)

// said is a server whose every tool says back the server and the tool called.
func said(_ context.Context, server, tool string, _ map[string]any) (string, bool, error) {
	return server + "/" + tool, false, nil
}

func TestABoxFindsAServersToolByTheNameTheModelCallsIt(t *testing.T) {
	schema := map[string]any{"type": "object"}
	box := Main.With([]Tool{ServerTool("my files", "read.file", "Reads a file.", schema, said)})
	definitions := box.Definitions()
	added := definitions[len(definitions)-1]
	if added.Name != "mcp_my_files_read_file" || added.Parameters["type"] != "object" || !strings.Contains(added.Description, "MCP server my files") {
		t.Fatalf("the model reads %+v", added)
	}
	found, has := box.Find("mcp_my_files_read_file")
	if !has || found.Name != protocol.ToolNameMcp {
		t.Fatalf("found %+v", found)
	}
	if _, has := box.Find(string(protocol.ToolNameMcp)); has {
		t.Fatal("mcp itself is no tool the model can call")
	}
	if _, has := Main.Find("mcp_my_files_read_file"); has {
		t.Fatal("the box it was made from has the server's tool too")
	}
	if kept, _ := box.Without(protocol.ToolNameAgent).Find("mcp_my_files_read_file"); kept.Function == "" {
		t.Fatal("a box without agent lost the server's tool")
	}
	if box.RunsTogether("mcp_my_files_read_file") {
		t.Fatal("a server's tool ran beside others: what it changes is not known")
	}
}

func TestAServersToolNameIsNoLongerThanAModelTakes(t *testing.T) {
	name := serverFunction(strings.Repeat("s", 50), strings.Repeat("t", 50))
	if len(name) != maxFunctionName {
		t.Fatalf("%d characters", len(name))
	}
}

func TestAServersToolIsAskedAboutByItsServerAndGivesWhatTheServerSaid(t *testing.T) {
	tool := ServerTool("files", "list", "Lists.", nil, said)
	ready := tool.Prepare(context.Background(), map[string]any{"path": "."}, &Context{})
	if !ready.OK || ready.Call.Subject != (permissions.Subject{Access: permissions.AccessServer, Server: "files"}) {
		t.Fatalf("prepared %+v", ready)
	}
	if ready.Summary != `files · list {"path":"."}` {
		t.Fatalf("summary %q", ready.Summary)
	}
	if outcome := ready.Call.Run(context.Background()); !outcome.OK || outcome.Output != "files/list" {
		t.Fatalf("outcome %+v", outcome)
	}
	if refused := tool.Prepare(context.Background(), "not an object", &Context{}); refused.OK || refused.Problem != notAnObject {
		t.Fatalf("a text went through: %+v", refused)
	}
}

func TestAServersToolThatFailsOrCannotBeCalledSaysSo(t *testing.T) {
	failing := ServerTool("files", "list", "Lists.", nil, func(context.Context, string, string, map[string]any) (string, bool, error) {
		return "No such folder.", true, nil
	})
	if outcome := failing.Prepare(context.Background(), map[string]any{}, &Context{}).Call.Run(context.Background()); outcome.OK || outcome.Output != "No such folder." {
		t.Fatalf("a tool that failed: %+v", outcome)
	}
	gone := ServerTool("files", "list", "Lists.", nil, func(context.Context, string, string, map[string]any) (string, bool, error) {
		return "", false, errors.New("the server stopped")
	})
	outcome := gone.Prepare(context.Background(), map[string]any{}, &Context{}).Call.Run(context.Background())
	if outcome.OK || !strings.Contains(outcome.Output, "the server stopped") {
		t.Fatalf("a server that is gone: %+v", outcome)
	}
	long := ServerTool("files", "read", "Reads.", nil, func(context.Context, string, string, map[string]any) (string, bool, error) {
		return strings.Repeat("x", maxServerOutput*2), false, nil
	})
	if outcome := long.Prepare(context.Background(), map[string]any{}, &Context{}).Call.Run(context.Background()); len(outcome.Output) > maxServerOutput+100 {
		t.Fatalf("%d characters reached the model", len(outcome.Output))
	}
}
