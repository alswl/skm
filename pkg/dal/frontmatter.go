package dal

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter is the parsed YAML header of a marker file.
type Frontmatter struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Version     *string `yaml:"version,omitempty"`
}

// HasFrontmatter reports whether data begins with a --- delimiter.
func HasFrontmatter(data []byte) bool {
	lines := strings.Split(string(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))), "\n")
	return len(lines) > 0 && strings.TrimSpace(lines[0]) == "---"
}

// ParseFrontmatter extracts the leading ---...--- YAML block from a marker
// file. When the file has no frontmatter it returns an empty Frontmatter and
// the full body. Malformed or unterminated frontmatter is an error.
func ParseFrontmatter(data []byte) (*Frontmatter, []byte, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return &Frontmatter{}, data, nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, nil, errors.New("unterminated frontmatter block")
	}
	yamlBlock := strings.Join(lines[1:end], "\n")
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return nil, nil, fmt.Errorf("invalid frontmatter: %w", err)
	}
	body := strings.Join(lines[end+1:], "\n")
	return &fm, []byte(body), nil
}

// EncodeFrontmatter renders a frontmatter block plus body back into a marker
// file.
func EncodeFrontmatter(fm *Frontmatter, body []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("---\n")
	enc := yaml.NewEncoder(&buf)
	_ = enc.Encode(fm)
	_ = enc.Close()
	buf.WriteString("---\n")
	buf.Write(body)
	return buf.Bytes()
}
