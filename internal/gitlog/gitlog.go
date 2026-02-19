package gitlog

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultLimit             = 20
	MaxLimit                 = 50
	DefaultMaxFilesPerCommit = 50
)

var (
	ErrNotGitRepo = errors.New("not a git repository")
)

type Reader interface {
	Recent(ctx context.Context, workDir string, opts Options) (Result, error)
}

type Options struct {
	Limit             int
	MaxFilesPerCommit int
}

type Result struct {
	Branch   string   `json:"branch"`
	Commits  []Commit `json:"commits"`
	Warnings []string `json:"-"`
}

type Commit struct {
	SHA            string      `json:"sha"`
	ShortSHA       string      `json:"short_sha"`
	CommittedAt    string      `json:"committed_at"`
	Subject        string      `json:"subject"`
	Stats          CommitStats `json:"stats"`
	Files          []FileStat  `json:"files"`
	FilesTruncated bool        `json:"files_truncated"`
}

type CommitStats struct {
	FilesChanged int  `json:"files_changed"`
	LinesAdded   int  `json:"lines_added"`
	LinesDeleted int  `json:"lines_deleted"`
	HasBinary    bool `json:"has_binary"`
}

type FileStat struct {
	Path    string `json:"path"`
	Added   *int   `json:"added"`
	Deleted *int   `json:"deleted"`
	Binary  bool   `json:"binary"`
}

type GitCmdReader struct{}

func (reader GitCmdReader) Recent(ctx context.Context, workDir string, opts Options) (Result, error) {
	normalized := normalizeOptions(opts)

	if _, err := runGit(ctx, workDir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return Result{}, classifyGitError(err)
	}

	branch, err := runGit(ctx, workDir, "branch", "--show-current")
	if err != nil || strings.TrimSpace(branch) == "" {
		branch, err = runGit(ctx, workDir, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return Result{}, classifyGitError(err)
		}
	}

	logOutput, err := runGit(
		ctx,
		workDir,
		"log",
		"-n",
		strconv.Itoa(normalized.Limit),
		"--date=iso-strict",
		"--pretty=format:%H%x00%cI%x00%s",
		"--numstat",
	)
	if err != nil {
		return Result{}, classifyGitError(err)
	}

	commits, warnings, err := ParseLogOutput(logOutput, normalized.MaxFilesPerCommit)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Branch:   strings.TrimSpace(branch),
		Commits:  commits,
		Warnings: warnings,
	}, nil
}

func normalizeOptions(opts Options) Options {
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	maxFiles := opts.MaxFilesPerCommit
	if maxFiles <= 0 {
		maxFiles = DefaultMaxFilesPerCommit
	}
	return Options{
		Limit:             limit,
		MaxFilesPerCommit: maxFiles,
	}
}

func runGit(ctx context.Context, workDir string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", workDir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func classifyGitError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		stderr := strings.ToLower(string(exitErr.Stderr))
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "not a git repository") || strings.Contains(stderr, "not a git repository") {
			return ErrNotGitRepo
		}
	}
	var notFoundErr *exec.Error
	if errors.As(err, &notFoundErr) && errors.Is(notFoundErr, exec.ErrNotFound) {
		return err
	}
	if strings.Contains(strings.ToLower(err.Error()), "not a git repository") {
		return ErrNotGitRepo
	}
	return err
}

func RecentWithTimeout(reader Reader, workDir string, opts Options, timeout time.Duration) (Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return reader.Recent(ctx, workDir, opts)
}
