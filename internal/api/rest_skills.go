package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gestalt/internal/terminal"
)

func (h *RestHandler) handleSkills(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	agentID := strings.TrimSpace(r.URL.Query().Get("agent"))
	metas := h.Manager.ListSkills()
	if agentID != "" {
		agentProfile, ok := h.Manager.GetAgent(agentID)
		if !ok {
			return &apiError{Status: http.StatusNotFound, Message: "agent not found"}
		}
		byName := make(map[string]terminal.SkillMetadata, len(metas))
		for _, meta := range metas {
			byName[meta.Name] = meta
		}
		filtered := make([]terminal.SkillMetadata, 0, len(agentProfile.Skills))
		for _, name := range agentProfile.Skills {
			if meta, ok := byName[name]; ok {
				filtered = append(filtered, meta)
			}
		}
		metas = filtered
	}

	response := make([]skillSummary, 0, len(metas))
	for _, meta := range metas {
		response = append(response, skillSummary{
			Name:          meta.Name,
			Description:   meta.Description,
			Path:          meta.Path,
			License:       meta.License,
			HasScripts:    hasSkillDir(meta.Path, "scripts"),
			HasReferences: hasSkillDir(meta.Path, "references"),
			HasAssets:     hasSkillDir(meta.Path, "assets"),
		})
	}

	writeJSON(w, http.StatusOK, response)
	return nil
}

func (h *RestHandler) handleSkill(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/skills/")
	name = strings.TrimSuffix(name, "/")
	if strings.TrimSpace(name) == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing skill name"}
	}

	entry, ok := h.Manager.GetSkill(name)
	if !ok || entry == nil {
		return &apiError{Status: http.StatusNotFound, Message: "skill not found"}
	}

	response := skillDetail{
		Name:          entry.Name,
		Description:   entry.Description,
		License:       entry.License,
		Compatibility: entry.Compatibility,
		Metadata:      entry.Metadata,
		AllowedTools:  entry.AllowedTools,
		Path:          entry.Path,
		Content:       entry.Content,
		Scripts:       listSkillFiles(entry.Path, "scripts"),
		References:    listSkillFiles(entry.Path, "references"),
		Assets:        listSkillFiles(entry.Path, "assets"),
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

func hasSkillDir(base, name string) bool {
	if strings.TrimSpace(base) == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(base, name))
	if err != nil {
		return false
	}
	return info.IsDir()
}

func listSkillFiles(base, name string) []string {
	if strings.TrimSpace(base) == "" {
		return nil
	}
	path := filepath.Join(base, name)
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	return files
}
