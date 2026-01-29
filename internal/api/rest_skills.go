package api

import (
	"net/http"
	"os"
	"path/filepath"
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
