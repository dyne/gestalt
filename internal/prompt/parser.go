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

const maxIncludeDepth = 3

var errBinaryInclude = errors.New("include file is binary")

type Parser struct {
	promptFS    fs.FS
	promptDir   string
	includeRoot string
}

type RenderResult struct {
	Content []byte
	Files   []string
}

func NewParser(promptFS fs.FS, promptDir, includeRoot string) *Parser {
	includeRoot = strings.TrimSpace(includeRoot)
	if includeRoot == "" {
		includeRoot = "."
	}
	return &Parser{
		promptFS:    promptFS,
		promptDir:   promptDir,
		includeRoot: includeRoot,
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
	result, found, err := p.renderFromCandidates(candidates, stack, p.readPromptFile)
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
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		data, err := p.readIncludeFile(candidate)
		if err != nil {
			if isNotExist(err) || errors.Is(err, errBinaryInclude) {
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

func (p *Parser) renderFromCandidates(candidates []string, stack []string, reader func(string) ([]byte, error)) (*RenderResult, bool, error) {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		data, err := reader(candidate)
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

func (p *Parser) readPromptFile(filename string) ([]byte, error) {
	if p.promptFS != nil {
		promptPath := path.Join(p.promptDir, filename)
		return fs.ReadFile(p.promptFS, promptPath)
	}
	promptPath := filepath.Join(p.promptDir, filename)
	return os.ReadFile(promptPath)
}

func (p *Parser) readIncludeFile(filename string) ([]byte, error) {
	cleaned, ok := cleanIncludePath(filename)
	if !ok {
		return nil, fs.ErrNotExist
	}
	data, err := readTextFile(filepath.Join(p.includeRoot, cleaned))
	if err == nil {
		return data, nil
	}
	if !isNotExist(err) && !errors.Is(err, errBinaryInclude) {
		return nil, err
	}

	if p.promptFS != nil {
		promptPath := path.Join(p.promptDir, cleaned)
		data, err := fs.ReadFile(p.promptFS, promptPath)
		if err != nil {
			return nil, err
		}
		if !isTextData(data) {
			return nil, errBinaryInclude
		}
		return data, nil
	}

	promptPath := filepath.Join(p.promptDir, cleaned)
	data, err = os.ReadFile(promptPath)
	if err != nil {
		return nil, err
	}
	if !isTextData(data) {
		return nil, errBinaryInclude
	}
	return data, nil
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
	cleaned := strings.TrimSpace(strings.ReplaceAll(includeName, "\\", "/"))
	if cleaned == "" {
		return nil
	}
	extension := strings.ToLower(path.Ext(cleaned))
	if extension == ".tmpl" || extension == ".txt" || extension == ".md" {
		return []string{cleaned}
	}
	if strings.Contains(cleaned, "/") {
		return []string{
			cleaned,
			cleaned + ".txt",
			cleaned + ".md",
		}
	}
	return []string{
		cleaned,
		cleaned + ".tmpl",
		cleaned + ".txt",
		cleaned + ".md",
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

func readTextFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !isTextData(data) {
		return nil, errBinaryInclude
	}
	return data, nil
}

func isTextData(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	nonPrintable := 0
	for _, b := range data {
		if b == 0 {
			return false
		}
		if b == '\n' || b == '\r' || b == '\t' {
			continue
		}
		if b < 0x20 {
			nonPrintable++
			continue
		}
		if b >= 0x7f && b < 0xa0 {
			nonPrintable++
			continue
		}
	}
	return nonPrintable*5 <= len(data)
}

func cleanIncludePath(name string) (string, bool) {
	cleaned := filepath.Clean(strings.TrimSpace(name))
	if cleaned == "." || cleaned == "" {
		return "", false
	}
	if filepath.IsAbs(cleaned) {
		return "", false
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(cleaned), true
}
