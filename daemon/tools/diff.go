package tools

import (
	"fmt"
	"strings"

	"swcode/daemon/protocol"
)

const (
	contextLines = 3
	// maxDiffLines: a whole new file can be long, the app shows its start, the model has the file anyway.
	maxDiffLines = 400
)

type operation int

const (
	keep operation = iota
	insert
	remove
)

type edit struct {
	operation operation
	text      string
}

// splitLines is a text's lines; a last line break ends the last line, it does not start one more.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// diffLines is the shortest list of edits from one list of lines to another (Myers' algorithm).
func diffLines(before, after []string) []edit {
	n, m := len(before), len(after)
	limit := n + m
	frontier := make([]int, 2*limit+2)
	trace := [][]int{}
	for depth := 0; depth <= limit; depth++ {
		trace = append(trace, append([]int(nil), frontier...))
		for diagonal := -depth; diagonal <= depth; diagonal += 2 {
			x := furthest(frontier, limit, diagonal, depth)
			y := x - diagonal
			for x < n && y < m && before[x] == after[y] {
				x, y = x+1, y+1
			}
			frontier[limit+diagonal] = x
			if x >= n && y >= m {
				return backtrack(trace, before, after, limit, depth)
			}
		}
	}
	return nil
}

// furthest is how far along before the diagonal can start this round: from the diagonal above or the one beside.
func furthest(frontier []int, limit, diagonal, depth int) int {
	if diagonal == -depth || (diagonal != depth && frontier[limit+diagonal-1] < frontier[limit+diagonal+1]) {
		return frontier[limit+diagonal+1]
	}
	return frontier[limit+diagonal-1] + 1
}

func backtrack(trace [][]int, before, after []string, limit, depth int) []edit {
	edits := []edit{}
	x, y := len(before), len(after)
	for round := depth; round > 0; round-- {
		frontier := trace[round]
		diagonal := x - y
		previous := diagonal + 1
		if diagonal == -round || (diagonal != round && frontier[limit+diagonal-1] < frontier[limit+diagonal+1]) {
			previous = diagonal + 1
		} else {
			previous = diagonal - 1
		}
		startX := frontier[limit+previous]
		startY := startX - previous
		for x > startX && y > startY {
			x, y = x-1, y-1
			edits = append(edits, edit{operation: keep, text: before[x]})
		}
		if x == startX {
			y--
			edits = append(edits, edit{operation: insert, text: after[y]})
		} else {
			x--
			edits = append(edits, edit{operation: remove, text: before[x]})
		}
	}
	for x > 0 && y > 0 {
		x, y = x-1, y-1
		edits = append(edits, edit{operation: keep, text: before[x]})
	}
	for left, right := 0, len(edits)-1; left < right; left, right = left+1, right-1 {
		edits[left], edits[right] = edits[right], edits[left]
	}
	return edits
}

// numbered is the edits as the app shows them, each line with its numbers on either side.
func numbered(edits []edit) []protocol.DiffLine {
	lines := make([]protocol.DiffLine, len(edits))
	oldLine, newLine := 1, 1
	for index, each := range edits {
		switch each.operation {
		case keep:
			lines[index] = protocol.DiffLine{Kind: protocol.DiffLineKindContext, NewLine: newLine, OldLine: oldLine, Text: each.text}
			oldLine, newLine = oldLine+1, newLine+1
		case insert:
			lines[index] = protocol.DiffLine{Kind: protocol.DiffLineKindAdd, NewLine: newLine, Text: each.text}
			newLine++
		case remove:
			lines[index] = protocol.DiffLine{Kind: protocol.DiffLineKindRemove, OldLine: oldLine, Text: each.text}
			oldLine++
		}
	}
	return lines
}

// hunks keeps the lines around each change, a gap between changes too far apart.
func hunks(lines []protocol.DiffLine) []protocol.DiffLine {
	shown := make([]bool, len(lines))
	for index, line := range lines {
		if line.Kind == protocol.DiffLineKindContext {
			continue
		}
		for near := max(0, index-contextLines); near <= min(len(lines)-1, index+contextLines); near++ {
			shown[near] = true
		}
	}
	kept := []protocol.DiffLine{}
	for index, line := range lines {
		if !shown[index] {
			continue
		}
		if len(kept) > 0 && !shown[index-1] {
			kept = append(kept, protocol.DiffLine{Kind: protocol.DiffLineKindGap})
		}
		kept = append(kept, line)
	}
	return kept
}

// FileDiff is the change to a file as the app shows it: numbered lines around each change, a gap between changes.
func FileDiff(path, before, after string) protocol.ToolDisplayDiff {
	lines := hunks(numbered(diffLines(splitLines(before), splitLines(after))))
	added, removed := 0, 0
	for _, line := range lines {
		switch line.Kind {
		case protocol.DiffLineKindAdd:
			added++
		case protocol.DiffLineKindRemove:
			removed++
		}
	}
	if len(lines) > maxDiffLines {
		more := len(lines) - maxDiffLines
		lines = append(lines[:maxDiffLines:maxDiffLines], protocol.DiffLine{Kind: protocol.DiffLineKindGap, Text: fmt.Sprintf("%d more lines", more)})
	}
	return protocol.ToolDisplayDiff{Added: added, Lines: lines, Path: path, Removed: removed}
}

func diffDisplay(diff protocol.ToolDisplayDiff) *protocol.ToolDisplay {
	return &protocol.ToolDisplay{Value: diff}
}
