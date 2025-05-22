package aiup_go_gen

import (
    "crypto/md5"
    "fmt"
    "regexp"
    "strings"
)

// CodeExtractor defines the interface for extracting code blocks from text.
type CodeExtractor interface {
    ExtractCodeBlocks(input string) []CodeBlock
}

// MarkdownCodeExtractor implements CodeExtractor for Markdown-formatted text.
type MarkdownCodeExtractor struct{}

// Regular expressions to identify code blocks and filename comments.
var (
    codeBlockRe       = regexp.MustCompile("(?s)```(\\w+)?\\s*\\n(.*?)\\n?```")
    filenameCommentRe = regexp.MustCompile(`(?m)^//\s*filename:\s*(.+)$`)
)

// ExtractCodeBlocks parses the input text and returns a slice of CodeBlocks.
func (m MarkdownCodeExtractor) ExtractCodeBlocks(input string) []CodeBlock {
    matches := codeBlockRe.FindAllStringSubmatch(input, -1)
    blocks := make([]CodeBlock, 0, len(matches))
    for _, match := range matches {
        language := strings.TrimSpace(match[1])
        code := strings.TrimSpace(match[2])
        filename := extractFilenameFromComment(code)
        if filename == "" {
            // Fallback: generate a filename based on a hash of the code.
            ext := languageToExt(language)
            hash := fmt.Sprintf("%x", md5.Sum([]byte(code)))[:8]
            filename = fmt.Sprintf("autogen_code_%s.%s", hash, ext)
        }
        blocks = append(blocks, CodeBlock{
            Language: language,
            Code:     code,
            Filename: filename,
        })
    }
    return blocks
}

// extractFilenameFromComment searches for a filename comment within the code block.
func extractFilenameFromComment(code string) string {
    m := filenameCommentRe.FindStringSubmatch(code)
    if len(m) > 1 {
        return strings.TrimSpace(m[1])
    }
    return ""
}

// languageToExt maps programming languages to their typical file extensions.
func languageToExt(lang string) string {
    switch strings.ToLower(lang) {
    case "go", "golang":
        return "go"
    case "python":
        return "py"
    case "bash", "shell", "sh":
        return "sh"
    case "javascript", "js":
        return "js"
    case "typescript", "ts":
        return "ts"
    default:
        return "txt"
    }
}