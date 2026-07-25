package tools

import (
	"fmt"
	"os"
	"sync"
)

// FileEdit records a reversible write/str_replace for /undo (Grok-style safety).
type FileEdit struct {
	Path       string
	Prev       []byte // previous content (empty if created)
	Created    bool   // file did not exist before
	Tool       string
}

var (
	undoMu   sync.Mutex
	undoStack []FileEdit
	maxUndo  = 20
)

// PushUndo records a snapshot before mutating a file.
func PushUndo(path string, prev []byte, created bool, tool string) {
	undoMu.Lock()
	defer undoMu.Unlock()
	undoStack = append(undoStack, FileEdit{
		Path: path, Prev: append([]byte(nil), prev...), Created: created, Tool: tool,
	})
	if len(undoStack) > maxUndo {
		undoStack = undoStack[len(undoStack)-maxUndo:]
	}
}

// UndoLast restores the most recent file edit. Returns a status message.
func UndoLast() (string, error) {
	undoMu.Lock()
	defer undoMu.Unlock()
	if len(undoStack) == 0 {
		return "", fmt.Errorf("nothing to undo")
	}
	ed := undoStack[len(undoStack)-1]
	undoStack = undoStack[:len(undoStack)-1]
	if ed.Created {
		if err := os.Remove(ed.Path); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("undo remove %s: %w", ed.Path, err)
		}
		return fmt.Sprintf("undid create %s (removed file)", ed.Path), nil
	}
	if err := os.WriteFile(ed.Path, ed.Prev, 0o644); err != nil {
		return "", fmt.Errorf("undo write %s: %w", ed.Path, err)
	}
	return fmt.Sprintf("undid %s on %s (%d bytes restored)", ed.Tool, ed.Path, len(ed.Prev)), nil
}

// UndoLen is number of reversible edits.
func UndoLen() int {
	undoMu.Lock()
	defer undoMu.Unlock()
	return len(undoStack)
}
