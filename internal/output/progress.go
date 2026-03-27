package output

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

// Progress tracks and displays review progress in the terminal.
type Progress struct {
	mu        sync.Mutex
	total     int
	completed int
	cached    int
	started   time.Time
	active    map[string]bool // currently in-flight file names
	enabled   bool
}

// NewProgress creates a new progress tracker.
func NewProgress(total int, enabled bool) *Progress {
	return &Progress{
		total:   total,
		started: time.Now(),
		active:  make(map[string]bool),
		enabled: enabled,
	}
}

// Start marks a file as in-progress.
func (p *Progress) Start(fileName string) {
	if !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active[fileName] = true
	p.render()
}

// Complete marks a file as done.
func (p *Progress) Complete(fileName string, wasCached bool) {
	if !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.active, fileName)
	p.completed++
	if wasCached {
		p.cached++
	}
	p.render()
}

// Finish clears the progress line and prints a newline.
func (p *Progress) Finish() {
	if !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	elapsed := time.Since(p.started).Round(time.Second)

	// Clear progress line and print final summary
	fmt.Print("\r\033[K")

	cyan := color.New(color.FgCyan)
	green := color.New(color.FgGreen)

	cyan.Print("  Agent review: ")
	green.Printf("%d/%d files", p.completed, p.total)
	if p.cached > 0 {
		fmt.Printf(" (%d from cache)", p.cached)
	}
	fmt.Printf(" in %s\n\n", elapsed)
}

func (p *Progress) render() {
	elapsed := time.Since(p.started).Round(time.Second)

	// Build progress bar: [████████░░░░] 8/41
	barWidth := 20
	filled := 0
	if p.total > 0 {
		filled = (p.completed * barWidth) / p.total
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	// Current file being reviewed
	currentFile := ""
	for f := range p.active {
		currentFile = f
		break
	}
	if len(currentFile) > 30 {
		currentFile = "..." + currentFile[len(currentFile)-27:]
	}

	cyan := color.New(color.FgCyan)
	green := color.New(color.FgGreen)

	// \r moves cursor to start of line, \033[K clears to end
	fmt.Print("\r\033[K")
	cyan.Print("  Reviewing ")
	fmt.Print("[")
	green.Print(bar)
	fmt.Printf("] %d/%d", p.completed, p.total)

	if p.cached > 0 {
		color.New(color.FgHiBlack).Printf(" (%d cached)", p.cached)
	}

	fmt.Printf("  %s", elapsed)

	if currentFile != "" {
		color.New(color.FgHiBlack).Printf("  %s", currentFile)
	}
}
