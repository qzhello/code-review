package model

// FileStatus represents the status of a file in a diff.
type FileStatus int

const (
	FileAdded    FileStatus = iota
	FileModified
	FileDeleted
	FileRenamed
)

func (s FileStatus) String() string {
	switch s {
	case FileAdded:
		return "added"
	case FileModified:
		return "modified"
	case FileDeleted:
		return "deleted"
	case FileRenamed:
		return "renamed"
	default:
		return "unknown"
	}
}

// LineType represents the type of a diff line.
type LineType int

const (
	LineContext LineType = iota
	LineAdded
	LineRemoved
)

// DiffResult holds the complete parsed diff.
type DiffResult struct {
	Files []FileDiff
}

// TotalChangedLines returns the total number of added + removed lines.
func (d *DiffResult) TotalChangedLines() int {
	total := 0
	for _, f := range d.Files {
		total += f.ChangedLines()
	}
	return total
}

// FileDiff represents changes to a single file.
type FileDiff struct {
	OldPath string
	NewPath string
	Status  FileStatus
	IsBinary bool
	Hunks   []Hunk
}

// Path returns the most relevant path (NewPath for renames/adds, OldPath for deletes).
func (f *FileDiff) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// ChangedLines returns the number of added + removed lines in this file.
func (f *FileDiff) ChangedLines() int {
	count := 0
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			if l.Type == LineAdded || l.Type == LineRemoved {
				count++
			}
		}
	}
	return count
}

// Hunk represents a single hunk in a unified diff.
type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Header   string // the @@ line
	Lines    []Line
}

// Line represents a single line in a diff hunk.
type Line struct {
	Type    LineType
	Content string
	OldNum  int // 0 if added
	NewNum  int // 0 if removed
}
