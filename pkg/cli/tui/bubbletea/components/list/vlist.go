// Package list provides virtualized list rendering for large datasets.
package list

import (
	"sync"
)

// VirtualList provides efficient rendering of large lists by only
// rendering visible items.
type VirtualList struct {
	mu sync.RWMutex

	// Data
	items    []ListItem
	itemKeys []string

	// Viewport
	height       int
	width        int
	scrollOffset int
	scrollAnchor int // Item to anchor scroll position

	// Heights (measured dynamically)
	itemHeights map[string]int

	// Estimated item height (fallback)
	estimatedHeight int

	// Selection
	selectedIndex int
	selectedID    string
}

// ListItem represents an item in the virtual list.
type ListItem interface {
	ID() string
	Render(width int) string
}

// NewVirtualList creates a new virtual list.
func NewVirtualList() *VirtualList {
	return &VirtualList{
		items:           []ListItem{},
		itemKeys:        []string{},
		itemHeights:     make(map[string]int),
		estimatedHeight: 1,
		scrollOffset:    0,
		selectedIndex:   -1,
	}
}

// SetItems sets the list items.
func (l *VirtualList) SetItems(items []ListItem) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.items = items
	l.itemKeys = make([]string, len(items))
	for i, item := range items {
		l.itemKeys[i] = item.ID()
	}

	// Preserve scroll position if possible
	if l.scrollAnchor >= len(items) {
		l.scrollAnchor = max(0, len(items)-1)
	}
	l.scrollOffset = 0 // Reset scroll, will be recalculated
}

// SetViewport sets the viewport dimensions.
func (l *VirtualList) SetViewport(height, width int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.height = height
	l.width = width
}

// SetEstimatedHeight sets the estimated item height for items not yet measured.
func (l *VirtualList) SetEstimatedHeight(h int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.estimatedHeight = h
}

// SetItemHeight updates the measured height for an item.
func (l *VirtualList) SetItemHeight(id string, height int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.itemHeights[id] = height
}

// VisibleItems returns the items visible in the current viewport.
func (l *VirtualList) VisibleItems() []VisibleItem {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if len(l.items) == 0 || l.height == 0 {
		return nil
	}

	var result []VisibleItem
	y := 0
	scrollY := l.calculateScrollY()

	// Skip items before scroll position
	itemIdx := 0
	accumulatedHeight := 0
	for itemIdx < len(l.items) && accumulatedHeight < scrollY {
		h := l.getItemHeight(itemIdx)
		accumulatedHeight += h
		itemIdx++
	}

	// Adjust for partial visibility
	if itemIdx > 0 && accumulatedHeight > scrollY {
		itemIdx--
		accumulatedHeight -= l.getItemHeight(itemIdx)
	}

	// Collect visible items
	for itemIdx < len(l.items) && y < l.height {
		item := l.items[itemIdx]
		h := l.getItemHeight(itemIdx)
		visibleHeight := h
		offsetY := 0

		// Handle partial visibility at top
		if accumulatedHeight < scrollY {
			offsetY = scrollY - accumulatedHeight
			visibleHeight -= offsetY
		}

		// Clip to viewport
		if y+visibleHeight > l.height {
			visibleHeight = l.height - y
		}

		if visibleHeight > 0 {
			result = append(result, VisibleItem{
				Index:       itemIdx,
				Item:        item,
				Y:           y,
				Height:      visibleHeight,
				OffsetY:     offsetY,
				IsSelected:  itemIdx == l.selectedIndex,
				TotalHeight: h,
			})
		}

		y += visibleHeight
		accumulatedHeight += h
		itemIdx++
	}

	return result
}

// VisibleItem represents an item visible in the viewport.
type VisibleItem struct {
	Index       int
	Item        ListItem
	Y           int // Y position in viewport
	Height      int // Visible height (may be clipped)
	OffsetY     int // Y offset within item (for partial visibility)
	IsSelected  bool
	TotalHeight int // Total height of the item
}

// ScrollTo scrolls to make the given item visible.
func (l *VirtualList) ScrollTo(index int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if index < 0 || index >= len(l.items) {
		return
	}

	l.scrollAnchor = index
	l.scrollOffset = 0 // Will be recalculated
}

// ScrollBy scrolls by the given number of items.
func (l *VirtualList) ScrollBy(delta int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	target := l.scrollAnchor + delta
	target = max(0, min(target, len(l.items)-1))
	l.scrollAnchor = target
	l.scrollOffset = 0
}

// ScrollToBottom scrolls to the bottom of the list.
func (l *VirtualList) ScrollToBottom() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.items) > 0 {
		l.scrollAnchor = len(l.items) - 1
		l.scrollOffset = l.getItemHeight(l.scrollAnchor)
	}
}

// Select selects the item at the given index.
func (l *VirtualList) Select(index int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if index < -1 || index >= len(l.items) {
		index = -1
	}
	l.selectedIndex = index
	if index >= 0 {
		l.selectedID = l.items[index].ID()
	} else {
		l.selectedID = ""
	}

	// Ensure selected item is visible
	if index >= 0 {
		l.scrollAnchor = index
	}
}

// SelectedIndex returns the currently selected item index.
func (l *VirtualList) SelectedIndex() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.selectedIndex
}

// TotalHeight returns the total height of all items.
func (l *VirtualList) TotalHeight() int {
	l.mu.RLock()
	defer l.mu.RUnlock()

	total := 0
	for i := range l.items {
		total += l.getItemHeight(i)
	}
	return total
}

// ItemCount returns the number of items.
func (l *VirtualList) ItemCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.items)
}

// calculateScrollY calculates the scroll position in pixels.
func (l *VirtualList) calculateScrollY() int {
	y := 0
	for i := 0; i < l.scrollAnchor && i < len(l.items); i++ {
		y += l.getItemHeight(i)
	}
	return y + l.scrollOffset
}

// getItemHeight returns the height for an item.
func (l *VirtualList) getItemHeight(index int) int {
	if index < 0 || index >= len(l.items) {
		return 0
	}
	key := l.itemKeys[index]
	if h, ok := l.itemHeights[key]; ok {
		return h
	}
	return l.estimatedHeight
}

// =============================================================================
// Scrollable List (with smooth scrolling)
// =============================================================================

// ScrollableList provides smooth scrolling with animation.
type ScrollableList struct {
	*VirtualList

	// Animation state
	targetScrollY  int
	currentScrollY int
	animating      bool
}

// NewScrollableList creates a new scrollable list.
func NewScrollableList() *ScrollableList {
	return &ScrollableList{
		VirtualList: NewVirtualList(),
	}
}

// ScrollToIndex scrolls to the given index with optional animation.
func (l *ScrollableList) ScrollToIndex(index int, animate bool) {
	if animate {
		// TODO: Implement smooth scrolling animation
		l.VirtualList.ScrollTo(index)
	} else {
		l.VirtualList.ScrollTo(index)
	}
}

// ScrollUp scrolls up by one item.
func (l *ScrollableList) ScrollUp() {
	l.ScrollBy(-1)
}

// ScrollDown scrolls down by one item.
func (l *ScrollableList) ScrollDown() {
	l.ScrollBy(1)
}
