package aiup_go_gen

// CodeBlock represents a ring-fenced code snippet extracted from markdown or other text.
// It includes metadata such as the language and an optional filename.
type CodeBlock struct {
    Language string // e.g. "python", "go", "bash"
    Code     string // The actual code snippet
    Filename string // Optional: the desired filename to write this code to
}