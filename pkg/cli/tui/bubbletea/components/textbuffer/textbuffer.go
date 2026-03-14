// Package textbuffer provides a multi-line text editor with cursor movement,
// history navigation, and auto-completion support.
package textbuffer

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Buffer represents a text buffer for multi-line editing.
type Buffer struct {
	mu sync.RWMutex

	// Text content stored as lines
	lines []string

	// Cursor position (row, col)
	cursorRow int
	cursorCol int

	// Visual scroll offset for display
	scrollRow int

	// Viewport dimensions
	viewportHeight int
	viewportWidth  int

	// Undo/redo stacks
	undoStack []EditAction
	redoStack []EditAction
	maxUndo   int

	// Paste content cache
	pasteCache map[string]string

	// History for navigation
	history    []string
	historyIdx int
}

// EditAction represents an edit operation for undo/redo.
type EditAction struct {
	Type     ActionType
	Row, Col int
	Text     string
	PrevText string
}

// ActionType is the type of edit action.
type ActionType int

const (
	ActionInsert ActionType = iota
	ActionDelete
	ActionReplace
)

// NewBuffer creates a new text buffer.
func NewBuffer() *Buffer {
	return &Buffer{
		lines:          []string{""},
		cursorRow:      0,
		cursorCol:      0,
		scrollRow:      0,
		viewportHeight: 10,
		viewportWidth:  80,
		undoStack:      make([]EditAction, 0, 100),
		redoStack:      make([]EditAction, 0, 100),
		maxUndo:        100,
		pasteCache:     make(map[string]string),
		history:        []string{},
		historyIdx:     -1,
	}
}

// =============================================================================
// Text Access
// =============================================================================

// Text returns the full text content.
func (b *Buffer) Text() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return strings.Join(b.lines, "\n")
}

// SetText sets the entire buffer content.
func (b *Buffer) SetText(text string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.lines = strings.Split(text, "\n")
	if len(b.lines) == 0 {
		b.lines = []string{""}
	}

	// Clamp cursor
	b.cursorRow = min(b.cursorRow, len(b.lines)-1)
	b.cursorCol = min(b.cursorCol, utf8.RuneCountInString(b.lines[b.cursorRow]))
}

// Line returns the text at the given row.
func (b *Buffer) Line(row int) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if row < 0 || row >= len(b.lines) {
		return ""
	}
	return b.lines[row]
}

// LineCount returns the number of lines.
func (b *Buffer) LineCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.lines)
}

// Lines returns all lines.
func (b *Buffer) Lines() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]string, len(b.lines))
	copy(result, b.lines)
	return result
}

// =============================================================================
// Cursor Position
// =============================================================================

// Cursor returns the current cursor position (row, col).
func (b *Buffer) Cursor() (int, int) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.cursorRow, b.cursorCol
}

// SetCursor sets the cursor position.
func (b *Buffer) SetCursor(row, col int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.cursorRow = clamp(row, 0, len(b.lines)-1)
	lineLen := utf8.RuneCountInString(b.lines[b.cursorRow])
	b.cursorCol = clamp(col, 0, lineLen)
	b.ensureCursorVisible()
}

// CursorOffset returns the cursor offset from the start of the text.
func (b *Buffer) CursorOffset() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	offset := 0
	for i := 0; i < b.cursorRow; i++ {
		offset += utf8.RuneCountInString(b.lines[i]) + 1 // +1 for newline
	}
	offset += b.cursorCol
	return offset
}

// SetCursorOffset sets the cursor from an offset.
func (b *Buffer) SetCursorOffset(offset int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	totalOffset := 0
	for row, line := range b.lines {
		lineLen := utf8.RuneCountInString(line)
		if totalOffset+lineLen >= offset {
			b.cursorRow = row
			b.cursorCol = offset - totalOffset
			b.ensureCursorVisible()
			return
		}
		totalOffset += lineLen + 1
	}

	// End of text
	b.cursorRow = len(b.lines) - 1
	b.cursorCol = utf8.RuneCountInString(b.lines[b.cursorRow])
	b.ensureCursorVisible()
}

// =============================================================================
// Editing Operations
// =============================================================================

// Insert inserts text at the current cursor position.
func (b *Buffer) Insert(text string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Save for undo
	prevText := b.lines[b.cursorRow]

	// Handle newlines
	if strings.Contains(text, "\n") {
		b.insertMultiLine(text)
		return
	}

	// Insert at cursor position
	line := b.lines[b.cursorRow]
	runes := []rune(line)
	before := runes[:b.cursorCol]
	after := runes[b.cursorCol:]
	newLine := string(append(before, []rune(text)...))
	newLine = string(append([]rune(newLine), after...))
	b.lines[b.cursorRow] = newLine
	b.cursorCol += utf8.RuneCountInString(text)

	// Save undo
	b.pushUndo(ActionInsert, b.cursorRow, b.cursorCol-utf8.RuneCountInString(text), text, prevText)
}

// insertMultiLine handles inserting text with newlines.
func (b *Buffer) insertMultiLine(text string) {
	lines := strings.Split(text, "\n")

	// Get current line parts
	currentLine := b.lines[b.cursorRow]
	runes := []rune(currentLine)
	before := runes[:b.cursorCol]
	after := runes[b.cursorCol:]

	// First line: combine before + first inserted line
	if len(lines) > 0 {
		b.lines[b.cursorRow] = string(before) + lines[0]
	}

	// Middle lines: insert as new lines
	for i := 1; i < len(lines)-1; i++ {
		b.insertLine(b.cursorRow+i, lines[i])
	}

	// Last line: combine last inserted line + after
	if len(lines) > 1 {
		lastLine := lines[len(lines)-1] + string(after)
		b.insertLine(b.cursorRow+len(lines)-1, lastLine)
		b.cursorRow += len(lines) - 1
		b.cursorCol = utf8.RuneCountInString(lines[len(lines)-1])
	} else {
		b.cursorCol += utf8.RuneCountInString(lines[0])
	}

	b.ensureCursorVisible()
}

// insertLine inserts a new line at the given position.
func (b *Buffer) insertLine(pos int, text string) {
	if pos < 0 || pos > len(b.lines) {
		return
	}
	b.lines = append(b.lines[:pos], append([]string{text}, b.lines[pos:]...)...)
}

// DeleteChar deletes the character before the cursor (backspace).
func (b *Buffer) DeleteChar() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cursorCol > 0 {
		// Delete from current line
		line := b.lines[b.cursorRow]
		runes := []rune(line)
		prevText := string(runes[b.cursorCol-1])
		b.lines[b.cursorRow] = string(append(runes[:b.cursorCol-1], runes[b.cursorCol:]...))
		b.cursorCol--
		b.pushUndo(ActionDelete, b.cursorRow, b.cursorCol, prevText, "")
	} else if b.cursorRow > 0 {
		// Merge with previous line
		prevLine := b.lines[b.cursorRow-1]
		prevLen := utf8.RuneCountInString(prevLine)
		b.lines[b.cursorRow-1] = prevLine + b.lines[b.cursorRow]
		b.lines = append(b.lines[:b.cursorRow], b.lines[b.cursorRow+1:]...)
		b.cursorRow--
		b.cursorCol = prevLen
		b.pushUndo(ActionDelete, b.cursorRow, b.cursorCol, "\n", "")
	}

	b.ensureCursorVisible()
}

// DeleteCharForward deletes the character at the cursor position.
func (b *Buffer) DeleteCharForward() {
	b.mu.Lock()
	defer b.mu.Unlock()

	line := b.lines[b.cursorRow]
	lineLen := utf8.RuneCountInString(line)

	if b.cursorCol < lineLen {
		// Delete from current line
		runes := []rune(line)
		prevText := string(runes[b.cursorCol])
		b.lines[b.cursorRow] = string(append(runes[:b.cursorCol], runes[b.cursorCol+1:]...))
		b.pushUndo(ActionDelete, b.cursorRow, b.cursorCol, prevText, "")
	} else if b.cursorRow < len(b.lines)-1 {
		// Merge with next line
		b.lines[b.cursorRow] = b.lines[b.cursorRow] + b.lines[b.cursorRow+1]
		b.lines = append(b.lines[:b.cursorRow+1], b.lines[b.cursorRow+2:]...)
		b.pushUndo(ActionDelete, b.cursorRow, b.cursorCol, "\n", "")
	}
}

// NewLine inserts a newline at the cursor position.
func (b *Buffer) NewLine() {
	b.mu.Lock()
	defer b.mu.Unlock()

	line := b.lines[b.cursorRow]
	runes := []rune(line)
	before := string(runes[:b.cursorCol])
	after := string(runes[b.cursorCol:])

	b.lines[b.cursorRow] = before
	b.insertLine(b.cursorRow+1, after)
	b.cursorRow++
	b.cursorCol = 0
	b.pushUndo(ActionInsert, b.cursorRow-1, len(runes), "\n", "")
	b.ensureCursorVisible()
}

// Clear clears the entire buffer.
func (b *Buffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.lines = []string{""}
	b.cursorRow = 0
	b.cursorCol = 0
	b.scrollRow = 0
}

// =============================================================================
// Cursor Movement
// =============================================================================

// MoveLeft moves the cursor left by one character.
func (b *Buffer) MoveLeft() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cursorCol > 0 {
		b.cursorCol--
	} else if b.cursorRow > 0 {
		b.cursorRow--
		b.cursorCol = utf8.RuneCountInString(b.lines[b.cursorRow])
	}
	b.ensureCursorVisible()
}

// MoveRight moves the cursor right by one character.
func (b *Buffer) MoveRight() {
	b.mu.Lock()
	defer b.mu.Unlock()

	lineLen := utf8.RuneCountInString(b.lines[b.cursorRow])
	if b.cursorCol < lineLen {
		b.cursorCol++
	} else if b.cursorRow < len(b.lines)-1 {
		b.cursorRow++
		b.cursorCol = 0
	}
	b.ensureCursorVisible()
}

// MoveUp moves the cursor up one line.
func (b *Buffer) MoveUp() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cursorRow > 0 {
		b.cursorRow--
		lineLen := utf8.RuneCountInString(b.lines[b.cursorRow])
		b.cursorCol = min(b.cursorCol, lineLen)
	}
	b.ensureCursorVisible()
}

// MoveDown moves the cursor down one line.
func (b *Buffer) MoveDown() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cursorRow < len(b.lines)-1 {
		b.cursorRow++
		lineLen := utf8.RuneCountInString(b.lines[b.cursorRow])
		b.cursorCol = min(b.cursorCol, lineLen)
	}
	b.ensureCursorVisible()
}

// MoveToStart moves the cursor to the start of the line.
func (b *Buffer) MoveToStart() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cursorCol = 0
	b.ensureCursorVisible()
}

// MoveToEnd moves the cursor to the end of the line.
func (b *Buffer) MoveToEnd() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cursorCol = utf8.RuneCountInString(b.lines[b.cursorRow])
	b.ensureCursorVisible()
}

// MoveToStartOfText moves the cursor to the start of the text.
func (b *Buffer) MoveToStartOfText() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cursorRow = 0
	b.cursorCol = 0
	b.scrollRow = 0
}

// MoveToEndOfText moves the cursor to the end of the text.
func (b *Buffer) MoveToEndOfText() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cursorRow = len(b.lines) - 1
	b.cursorCol = utf8.RuneCountInString(b.lines[b.cursorRow])
	b.ensureCursorVisible()
}

// MoveWordLeft moves the cursor left by one word.
func (b *Buffer) MoveWordLeft() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Skip spaces
	for b.cursorCol > 0 {
		r, _ := b.runeAt(b.cursorRow, b.cursorCol-1)
		if !unicode.IsSpace(r) {
			break
		}
		b.cursorCol--
	}

	// Skip word
	for b.cursorCol > 0 {
		r, _ := b.runeAt(b.cursorRow, b.cursorCol-1)
		if unicode.IsSpace(r) {
			break
		}
		b.cursorCol--
	}

	b.ensureCursorVisible()
}

// MoveWordRight moves the cursor right by one word.
func (b *Buffer) MoveWordRight() {
	b.mu.Lock()
	defer b.mu.Unlock()

	lineLen := utf8.RuneCountInString(b.lines[b.cursorRow])

	// Skip current word
	for b.cursorCol < lineLen {
		r, _ := b.runeAt(b.cursorRow, b.cursorCol)
		if unicode.IsSpace(r) {
			break
		}
		b.cursorCol++
	}

	// Skip spaces
	for b.cursorCol < lineLen {
		r, _ := b.runeAt(b.cursorRow, b.cursorCol)
		if !unicode.IsSpace(r) {
			break
		}
		b.cursorCol++
	}

	b.ensureCursorVisible()
}

// DeleteWordLeft deletes the word before the cursor.
func (b *Buffer) DeleteWordLeft() {
	b.mu.Lock()
	defer b.mu.Unlock()

	startCol := b.cursorCol

	// Skip spaces
	for b.cursorCol > 0 {
		r, _ := b.runeAt(b.cursorRow, b.cursorCol-1)
		if !unicode.IsSpace(r) {
			break
		}
		b.cursorCol--
	}

	// Skip word
	for b.cursorCol > 0 {
		r, _ := b.runeAt(b.cursorRow, b.cursorCol-1)
		if unicode.IsSpace(r) {
			break
		}
		b.cursorCol--
	}

	// Delete range
	if startCol != b.cursorCol {
		line := b.lines[b.cursorRow]
		runes := []rune(line)
		deleted := string(runes[b.cursorCol:startCol])
		b.lines[b.cursorRow] = string(append(runes[:b.cursorCol], runes[startCol:]...))
		b.pushUndo(ActionDelete, b.cursorRow, b.cursorCol, deleted, "")
	}

	b.ensureCursorVisible()
}

// =============================================================================
// Viewport
// =============================================================================

// SetViewport sets the viewport dimensions.
func (b *Buffer) SetViewport(height, width int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.viewportHeight = height
	b.viewportWidth = width
	b.ensureCursorVisible()
}

// VisualLines returns the visible lines for rendering.
func (b *Buffer) VisualLines() []VisualLine {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]VisualLine, 0, b.viewportHeight)
	endRow := min(b.scrollRow+b.viewportHeight, len(b.lines))

	for row := b.scrollRow; row < endRow; row++ {
		line := b.lines[row]
		vl := VisualLine{
			Row:      row,
			Text:     line,
			IsCursor: row == b.cursorRow,
		}

		if vl.IsCursor {
			vl.CursorCol = b.cursorCol
		}

		result = append(result, vl)
	}

	return result
}

// VisualLine represents a line for visual rendering.
type VisualLine struct {
	Row       int
	Text      string
	IsCursor  bool
	CursorCol int
}

// ensureCursorVisible ensures the cursor is visible in the viewport.
func (b *Buffer) ensureCursorVisible() {
	// Vertical scroll
	if b.cursorRow < b.scrollRow {
		b.scrollRow = b.cursorRow
	} else if b.cursorRow >= b.scrollRow+b.viewportHeight {
		b.scrollRow = b.cursorRow - b.viewportHeight + 1
	}
}

// =============================================================================
// History (Input History)
// =============================================================================

// AddToHistory adds the current text to history.
func (b *Buffer) AddToHistory() {
	b.mu.Lock()
	defer b.mu.Unlock()

	text := strings.Join(b.lines, "\n")
	if text == "" {
		return
	}

	// Don't add duplicates
	if len(b.history) > 0 && b.history[len(b.history)-1] == text {
		return
	}

	b.history = append(b.history, text)
	b.historyIdx = len(b.history)
}

// HistoryUp navigates up in history (older entries).
func (b *Buffer) HistoryUp() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.historyIdx > 0 {
		b.historyIdx--
		b.lines = strings.Split(b.history[b.historyIdx], "\n")
		if len(b.lines) == 0 {
			b.lines = []string{""}
		}
		b.cursorRow = len(b.lines) - 1
		b.cursorCol = utf8.RuneCountInString(b.lines[b.cursorRow])
		return true
	}
	return false
}

// HistoryDown navigates down in history (newer entries).
func (b *Buffer) HistoryDown() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.historyIdx < len(b.history)-1 {
		b.historyIdx++
		b.lines = strings.Split(b.history[b.historyIdx], "\n")
		if len(b.lines) == 0 {
			b.lines = []string{""}
		}
		b.cursorRow = len(b.lines) - 1
		b.cursorCol = utf8.RuneCountInString(b.lines[b.cursorRow])
		return true
	} else if b.historyIdx == len(b.history)-1 {
		b.historyIdx = len(b.history)
		b.lines = []string{""}
		b.cursorRow = 0
		b.cursorCol = 0
		return true
	}
	return false
}

// =============================================================================
// Undo/Redo
// =============================================================================

// Undo undoes the last action.
func (b *Buffer) Undo() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.undoStack) == 0 {
		return false
	}

	action := b.undoStack[len(b.undoStack)-1]
	b.undoStack = b.undoStack[:len(b.undoStack)-1]

	// Apply reverse
	// TODO: Implement actual undo logic

	b.redoStack = append(b.redoStack, action)
	return true
}

// Redo redoes the last undone action.
func (b *Buffer) Redo() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.redoStack) == 0 {
		return false
	}

	action := b.redoStack[len(b.redoStack)-1]
	b.redoStack = b.redoStack[:len(b.redoStack)-1]

	// Apply action
	// TODO: Implement actual redo logic

	b.undoStack = append(b.undoStack, action)
	return true
}

// pushUndo pushes an action to the undo stack.
func (b *Buffer) pushUndo(typ ActionType, row, col int, text, prevText string) {
	action := EditAction{
		Type:     typ,
		Row:      row,
		Col:      col,
		Text:     text,
		PrevText: prevText,
	}

	b.undoStack = append(b.undoStack, action)
	if len(b.undoStack) > b.maxUndo {
		b.undoStack = b.undoStack[1:]
	}

	// Clear redo stack
	b.redoStack = b.redoStack[:0]
}

// =============================================================================
// Helper Functions
// =============================================================================

// runeAt returns the rune at the given position.
func (b *Buffer) runeAt(row, col int) (rune, bool) {
	if row < 0 || row >= len(b.lines) {
		return 0, false
	}
	runes := []rune(b.lines[row])
	if col < 0 || col >= len(runes) {
		return 0, false
	}
	return runes[col], true
}

// clamp clamps a value between min and max.
func clamp(val, minVal, maxVal int) int {
	return max(minVal, min(maxVal, val))
}
