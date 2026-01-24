package git

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveGitDir resolves the .git directory path for a working directory.
func ResolveGitDir(workDir string) string {
	gitPath := filepath.Join(workDir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return gitPath
	}
	if !info.Mode().IsRegular() {
		return ""
	}
	contents, err := os.ReadFile(gitPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(contents))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return ""
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if gitDir == "" {
		return ""
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(workDir, gitDir)
	}
	return gitDir
}

// ReadGitOrigin reads the origin URL from a git config file.
func ReadGitOrigin(configPath string) string {
	file, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	section := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if section != `remote "origin"` {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if key != "url" {
			continue
		}
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// ReadGitBranch reads the branch name or detached HEAD from a git HEAD file.
func ReadGitBranch(headPath string) string {
	contents, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(contents))
	if line == "" {
		return ""
	}
	const prefix = "ref: "
	if strings.HasPrefix(line, prefix) {
		ref := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	short := line
	if len(short) > 12 {
		short = short[:12]
	}
	return fmt.Sprintf("detached@%s", short)
}
