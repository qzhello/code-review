package git

import (
	"testing"

	"github.com/quzhihao/code-review/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleDiff = `diff --git a/main.go b/main.go
index 1234567..abcdefg 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main

 import "fmt"
+import "os"

 func main() {
@@ -10,3 +11,5 @@ func main() {
 	fmt.Println("hello")
+	fmt.Println("world")
+	os.Exit(0)
 }
`

func TestParseDiff(t *testing.T) {
	result := ParseDiff(sampleDiff)

	require.Len(t, result.Files, 1)

	f := result.Files[0]
	assert.Equal(t, "main.go", f.OldPath)
	assert.Equal(t, "main.go", f.NewPath)
	assert.Equal(t, model.FileModified, f.Status)
	assert.False(t, f.IsBinary)
	require.Len(t, f.Hunks, 2)

	// First hunk: 1 added line
	h1 := f.Hunks[0]
	assert.Equal(t, 1, h1.OldStart)
	assert.Equal(t, 5, h1.OldCount)
	assert.Equal(t, 1, h1.NewStart)
	assert.Equal(t, 6, h1.NewCount)

	addedCount := 0
	for _, l := range h1.Lines {
		if l.Type == model.LineAdded {
			addedCount++
			assert.Contains(t, l.Content, `import "os"`)
		}
	}
	assert.Equal(t, 1, addedCount)

	// Second hunk: 2 added lines
	h2 := f.Hunks[1]
	assert.Equal(t, 10, h2.OldStart)
	assert.Equal(t, 11, h2.NewStart)

	addedCount = 0
	for _, l := range h2.Lines {
		if l.Type == model.LineAdded {
			addedCount++
		}
	}
	assert.Equal(t, 2, addedCount)

	// Total changed lines
	assert.Equal(t, 3, result.TotalChangedLines())
}

const newFileDiff = `diff --git a/newfile.go b/newfile.go
new file mode 100644
index 0000000..1234567
--- /dev/null
+++ b/newfile.go
@@ -0,0 +1,3 @@
+package main
+
+func NewFunc() {}
`

func TestParseDiff_NewFile(t *testing.T) {
	result := ParseDiff(newFileDiff)

	require.Len(t, result.Files, 1)
	f := result.Files[0]
	assert.Equal(t, model.FileAdded, f.Status)
	assert.Equal(t, "newfile.go", f.Path())
	assert.Equal(t, 3, f.ChangedLines())
}

const deletedFileDiff = `diff --git a/old.go b/old.go
deleted file mode 100644
index 1234567..0000000
--- a/old.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package main
-
-func OldFunc() {}
`

func TestParseDiff_DeletedFile(t *testing.T) {
	result := ParseDiff(deletedFileDiff)

	require.Len(t, result.Files, 1)
	f := result.Files[0]
	assert.Equal(t, model.FileDeleted, f.Status)
	assert.Equal(t, "old.go", f.OldPath)
	assert.Equal(t, 3, f.ChangedLines())
}

func TestParseDiff_Empty(t *testing.T) {
	result := ParseDiff("")
	assert.Len(t, result.Files, 0)
}

const multiFileDiff = `diff --git a/a.go b/a.go
index 1234567..abcdefg 100644
--- a/a.go
+++ b/a.go
@@ -1,3 +1,4 @@
 package a
+// comment

 func A() {}
diff --git a/b.go b/b.go
index 1234567..abcdefg 100644
--- a/b.go
+++ b/b.go
@@ -1,3 +1,3 @@
 package b

-func B() {}
+func B() int { return 0 }
`

func TestParseDiff_MultipleFiles(t *testing.T) {
	result := ParseDiff(multiFileDiff)

	require.Len(t, result.Files, 2)
	assert.Equal(t, "a.go", result.Files[0].Path())
	assert.Equal(t, "b.go", result.Files[1].Path())
	assert.Equal(t, 1, result.Files[0].ChangedLines())
	assert.Equal(t, 2, result.Files[1].ChangedLines()) // 1 removed + 1 added
}
