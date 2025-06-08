package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

)

func CreateTempDir() string {
    tmp, _ := ioutil.TempDir("", "exec")
    return tmp
}

func WriteTempFile(dir, filename, content string) error {
    path := filepath.Join(dir, filename)
    return ioutil.WriteFile(path, []byte(content), 0644)
}

func RemoveTempDir(dir string) {
    os.RemoveAll(dir)
}

func CodeBlockRegex() *regexp.Regexp {
	return regexp.MustCompile("(?s)```\\s*([a-zA-Z0-9_-]*)\\s*\\n(.*?)\\s*```")
}

func ExtractCodeBlocks(code string) []struct {
    Language string
    FileName string
    Content  string
} {
    fmt.Println("Extracting code blocks from response:", code[:20], "...")
    re := regexp.MustCompile("(?s)```\\s*([a-zA-Z0-9_-]*)\\s*\\n(.*?)\\s*```")
    matches := re.FindAllStringSubmatch(code, -1)
    var blocks []struct {
        Language string
        FileName string
        Content  string
    }
    for _, m := range matches {
        lang := strings.TrimSpace(m[1])
        block := strings.TrimSpace(m[2])

        // Check for # filename: ... as the first line
        lines := strings.SplitN(block, "\n", 2)
        fileName := ""
        content := block
        if len(lines) > 0 && strings.HasPrefix(lines[0], "# filename:") {
            fileName = strings.TrimSpace(strings.TrimPrefix(lines[0], "# filename:"))
            if len(lines) > 1 {
                content = lines[1]
            } else {
                content = ""
            }
        }
        blocks = append(blocks, struct {
            Language string
            FileName string
            Content  string
        }{
            Language: lang,
            FileName: fileName,
            Content:  content,
        })
    }
    // Fallback if nothing extracted
    if len(blocks) == 0 && strings.TrimSpace(code) != "" {
        blocks = append(blocks, struct {
            Language string
            FileName string
            Content  string
        }{"", "", code})
    }
    fmt.Println("Extracted code blocks:", len(blocks))
    return blocks
}