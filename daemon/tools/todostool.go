package tools

import (
	"context"
	"strings"

	"swcode/daemon/protocol"
)

const (
	todoOpenMark = "[ ]"
	todoDoneMark = "[x]"
	noTodos      = "Todos are not available here: this session works in no project."
)

type todoInput struct {
	Text string `json:"text"`
}

// todoText is the project's todos as the model reads them: a line per todo, done or not.
func todoText(todos []protocol.ProjectTodo) string {
	lines := make([]string, len(todos))
	for index, todo := range todos {
		mark := todoOpenMark
		if todo.Done {
			mark = todoDoneMark
		}
		lines[index] = mark + " " + todo.Text
	}
	return strings.Join(lines, "\n")
}

// todoOutcome shows the project's todos after a change, so the model knows what is left.
func todoOutcome(todos []protocol.ProjectTodo, meta, said string) Outcome {
	text := said + "\nThe project’s todos:\n" + todoText(todos)
	return Outcome{Display: Text(text), Meta: meta, OK: true, Output: text}
}

// todoFailure shows the project's todos too, so the model corrects a text that matches none.
func todoFailure(problem string, todos []protocol.ProjectTodo, meta string) Outcome {
	if len(todos) == 0 {
		return Failure(problem, meta)
	}
	return Failure(problem+"\nThe project’s todos:\n"+todoText(todos), meta)
}

// todoTool opens a todo on the project: work the request needs that neither the project's nor the session's pinned
// todos say.
var todoTool = Define(Spec[todoInput]{
	Check: func(_ context.Context, input todoInput, tc *Context) Check {
		if tc.Todos == nil {
			return Refuse(noTodos)
		}
		return Ready(func(context.Context) Outcome {
			todos, problem := tc.Todos.OpenTodo(tc.Workspace, tc.HostRef, input.Text)
			if problem != "" {
				return todoFailure(problem, todos, "not opened")
			}
			return todoOutcome(todos, "opened", "Opened “"+input.Text+"” on the project.")
		}, nil)
	},
	Description: "Opens a todo on the project, at the end of its list: work the request needs that neither the project’s todos nor this session’s pinned todos say. The user sees it in the project’s Todo tab. Close it with closetodo when it is done.",
	Input:       Object(Required("text", String().Trimmed().Min(1).Max(protocol.MaxTodoText).Describe("What the todo says, in a few words."))),
	Name:        protocol.ToolNameTodo,
	Subject:     reads,
	Summarize: func(input todoInput) string {
		return input.Text
	},
})

// closeTodoTool closes a todo of the project, by what it says: a pinned todo of the session, or one the agent opened.
var closeTodoTool = Define(Spec[todoInput]{
	Check: func(_ context.Context, input todoInput, tc *Context) Check {
		if tc.Todos == nil {
			return Refuse(noTodos)
		}
		return Ready(func(context.Context) Outcome {
			todos, problem := tc.Todos.CloseTodo(tc.Workspace, tc.HostRef, input.Text)
			if problem != "" {
				return todoFailure(problem, todos, "not closed")
			}
			return todoOutcome(todos, "closed", "Closed “"+input.Text+"”.")
		}, nil)
	},
	Description: "Closes a todo of the project, one that is there: a todo pinned to this session, or one you opened with todo. The text must be what the todo says. Close each as soon as its work is done, so the project’s Todo tab shows what is left.",
	Input:       Object(Required("text", String().Trimmed().Min(1).Max(protocol.MaxTodoText).Describe("What the todo says, word for word."))),
	Name:        protocol.ToolNameClosetodo,
	Subject:     reads,
	Summarize: func(input todoInput) string {
		return input.Text
	},
})
