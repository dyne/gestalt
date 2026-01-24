package app

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"gestalt/internal/agent"
	"gestalt/internal/logging"
	"gestalt/internal/skill"
	"gestalt/internal/temporal"
	"gestalt/internal/terminal"
)

type BuildOptions struct {
	Logger               *logging.Logger
	Shell                string
	ConfigFS             fs.FS
	ConfigOverlay        fs.FS
	ConfigRoot           string
	AgentsDir            string
	TemporalClient       temporal.WorkflowClient
	TemporalEnabled      bool
	SessionLogDir        string
	InputHistoryDir      string
	SessionRetentionDays int
	BufferLines          int
}

type BuildResult struct {
	Manager *terminal.Manager
	Skills  map[string]*skill.Skill
	Agents  map[string]agent.Agent
}

type BuildError struct {
	Stage string
	Err   error
}

func (e BuildError) Error() string {
	if e.Err == nil {
		return e.Stage
	}
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

func (e BuildError) Unwrap() error {
	return e.Err
}

const (
	StageLoadSkills = "load_skills"
	StageLoadAgents = "load_agents"
)

func Build(options BuildOptions) (*BuildResult, error) {
	if options.ConfigFS == nil {
		return nil, errors.New("config filesystem is required")
	}
	if strings.TrimSpace(options.ConfigRoot) == "" {
		return nil, errors.New("config root is required")
	}
	if strings.TrimSpace(options.AgentsDir) == "" {
		return nil, errors.New("agents directory is required")
	}

	configOverlay := options.ConfigOverlay
	if configOverlay == nil {
		configOverlay = options.ConfigFS
	}

	skills, err := LoadSkills(options.Logger, options.ConfigFS, options.ConfigRoot)
	if err != nil {
		return nil, BuildError{Stage: StageLoadSkills, Err: err}
	}

	agents, err := LoadAgents(options.Logger, configOverlay, options.ConfigRoot, BuildSkillIndex(skills))
	if err != nil {
		return nil, BuildError{Stage: StageLoadAgents, Err: err}
	}

	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:                options.Shell,
		Agents:               agents,
		AgentsDir:            options.AgentsDir,
		Skills:               skills,
		Logger:               options.Logger,
		TemporalClient:       options.TemporalClient,
		TemporalEnabled:      options.TemporalEnabled,
		SessionLogDir:        options.SessionLogDir,
		InputHistoryDir:      options.InputHistoryDir,
		SessionRetentionDays: options.SessionRetentionDays,
		BufferLines:          options.BufferLines,
		PromptFS:             configOverlay,
		PromptDir:            path.Join(options.ConfigRoot, "prompts"),
	})

	return &BuildResult{
		Manager: manager,
		Skills:  skills,
		Agents:  agents,
	}, nil
}
