package prompt

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const maxIncludeDepth = 10

type Parser struct {
	promptFS  fs.FS
	promptDir string
}

type RenderResult struct {
	Content []byte
	Files   []string
}

func NewParser(promptFS fs.FS, promptDir string) *Parser {
	return &Parser{
		promptFS:  promptFS,
		promptDir: promptDir,
	}
}

func (p *Parser) Render(promptName string) (*RenderResult, error) {
	promptName = strings.TrimSpace(promptName)
	if promptName == "" {
		return nil, errors.New("prompt name is required")
	}
	return p.renderPrompt(promptName, nil)
}

func (p *Parser) renderPrompt(promptName string, stack []string) (*RenderResult, error) {
	candidates := promptCandidates(promptName)
	result, found, err := p.renderFromCandidates(candidates, stack)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("prompt %q not found", promptName)
	}
	return result, nil
}

func (p *Parser) renderInclude(includeName string, stack []string) (*RenderResult, bool, error) {
	candidates := includeCandidates(includeName)
	result, found, err := p.renderFromCandidates(candidates, stack)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return result, true, nil
}

func (p *Parser) renderFromCandidates(candidates []string, stack []string) (*RenderResult, bool, error) {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		data, err := p.readFile(candidate)
		if err != nil {
			if isNotExist(err) {
				continue
			}
			return nil, false, err
		}
		result, err := p.renderFile(candidate, data, stack)
		if err != nil {
			return nil, false, err
		}
		return result, true, nil
	}
	return nil, false, nil
}

func (p *Parser) renderFile(filename string, data []byte, stack []string) (*RenderResult, error) {
	updatedStack, err := pushStack(filename, stack)
	if err != nil {
		return nil, err
	}
	files := []string{filename}
	if !strings.HasSuffix(strings.ToLower(filename), ".tmpl") {
		return &RenderResult{
			Content: data,
			Files:   files,
		}, nil
	}

	reader := bufio.NewReader(bytes.NewReader(data))
	var output bytes.Buffer
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		if line != "" {
			if includeName, ok := parseIncludeDirective(line); ok {
				includeResult, found, includeErr := p.renderInclude(includeName, updatedStack)
				if includeErr != nil {
					return nil, includeErr
				}
				if found {
					output.Write(includeResult.Content)
					files = append(files, includeResult.Files...)
				}
			} else {
				output.WriteString(line)
			}
		}
		if readErr == io.EOF {
			break
		}
	}

	return &RenderResult{
		Content: output.Bytes(),
		Files:   files,
	}, nil
}

func (p *Parser) readFile(filename string) ([]byte, error) {
	if p.promptFS != nil {
		promptPath := path.Join(p.promptDir, filename)
		return fs.ReadFile(p.promptFS, promptPath)
	}
	promptPath := filepath.Join(p.promptDir, filename)
	return os.ReadFile(promptPath)
}

func promptCandidates(promptName string) []string {
	extension := strings.ToLower(path.Ext(promptName))
	if extension == ".tmpl" || extension == ".txt" {
		return []string{promptName}
	}
	return []string{
		promptName + ".tmpl",
		promptName + ".txt",
	}
}

func includeCandidates(includeName string) []string {
	extension := strings.ToLower(path.Ext(includeName))
	if extension == ".tmpl" || extension == ".txt" {
		return []string{includeName}
	}
	return []string{
		includeName,
		includeName + ".txt",
	}
}

func parseIncludeDirective(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "{{") || !strings.HasSuffix(trimmed, "}}") {
		return "", false
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "{{"), "}}"))
	if inner == "" {
		return "", false
	}
	fields := strings.Fields(inner)
	if len(fields) < 2 || fields[0] != "include" {
		return "", false
	}
	includeName := strings.Join(fields[1:], " ")
	if includeName == "" {
		return "", false
	}
	return includeName, true
}

func pushStack(filename string, stack []string) ([]string, error) {
	if len(stack) >= maxIncludeDepth {
		chain := append(stack, filename)
		return nil, fmt.Errorf("prompt include depth exceeded (%d): %s", maxIncludeDepth, strings.Join(chain, " -> "))
	}
	for _, entry := range stack {
		if entry == filename {
			chain := append(stack, filename)
			return nil, fmt.Errorf("prompt include cycle detected: %s", strings.Join(chain, " -> "))
		}
	}
	return append(stack, filename), nil
}

func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err)
}
