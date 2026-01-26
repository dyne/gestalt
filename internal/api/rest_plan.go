package api

import (
	"net/http"

	"gestalt/internal/plan"
)

func (h *RestHandler) handlePlansList(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	plans, err := plan.ScanPlansDirectory(plan.DefaultPlansDir())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Warn("plans scan failed", map[string]string{
				"error": err.Error(),
			})
		}
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to read plans"}
	}

	response := plansListResponse{Plans: mapPlanDocuments(plans)}
	writeJSON(w, http.StatusOK, response)
	return nil
}

func mapPlanDocuments(source []plan.PlanDocument) []planDocument {
	if len(source) == 0 {
		return []planDocument{}
	}
	result := make([]planDocument, 0, len(source))
	for _, doc := range source {
		result = append(result, planDocument{
			Filename:  doc.Filename,
			Title:     doc.Metadata.Title,
			Subtitle:  doc.Metadata.Subtitle,
			Date:      doc.Metadata.Date,
			Keywords:  doc.Metadata.Keywords,
			Headings:  mapPlanHeadings(doc.Headings),
			L1Count:   doc.L1Count,
			L2Count:   doc.L2Count,
			PriorityA: doc.PriorityA,
			PriorityB: doc.PriorityB,
			PriorityC: doc.PriorityC,
		})
	}
	return result
}

func mapPlanHeadings(source []plan.Heading) []planHeading {
	if len(source) == 0 {
		return []planHeading{}
	}
	result := make([]planHeading, 0, len(source))
	for _, heading := range source {
		result = append(result, planHeading{
			Level:    heading.Level,
			Keyword:  heading.Keyword,
			Priority: heading.Priority,
			Text:     heading.Text,
			Body:     heading.Body,
			Children: mapPlanHeadings(heading.Children),
		})
	}
	return result
}
