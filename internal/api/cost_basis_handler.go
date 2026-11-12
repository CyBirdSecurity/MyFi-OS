package api

import (
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
)

// handleBulkUpdateCostBasis accepts a list of {id, costBasis} pairs and updates each holding.
func (s *Server) handleBulkUpdateCostBasis(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	var body []struct {
		ID        string   `json:"id"`
		CostBasis *float64 `json:"costBasis"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	updates := make([]db.CostBasisUpdate, 0, len(body))
	for _, item := range body {
		id, err := uuid.Parse(item.ID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "invalid holding id: "+item.ID)
			return
		}
		updates = append(updates, db.CostBasisUpdate{ID: id, CostBasis: item.CostBasis})
	}
	if err := s.holdings.BulkUpdateCostBasis(r.Context(), userID, updates); err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCostBasisImportPreview parses a broker portfolio CSV and returns detected
// column mapping plus matching holdings for each row.
func (s *Server) handleCostBasisImportPreview(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "file too large or not multipart")
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "missing file field")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "read file")
		return
	}

	// Build account name lookup from the user's accounts.
	accounts, err := s.accounts.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	accountNames := make(map[uuid.UUID]string, len(accounts))
	for _, a := range accounts {
		accountNames[a.ID] = a.Name
	}

	result, err := s.costBasisImport.PreviewFull(r.Context(), userID, data, accountNames)
	if err != nil {
		writeError(w, http.StatusBadRequest, "PARSE_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleCostBasisImportApply commits the user-approved cost basis values.
func (s *Server) handleCostBasisImportApply(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	var body struct {
		Rows []struct {
			HoldingID string  `json:"holdingId"`
			CostBasis float64 `json:"costBasis"`
		} `json:"rows"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	updates := make([]db.CostBasisUpdate, 0, len(body.Rows))
	for _, row := range body.Rows {
		id, err := uuid.Parse(row.HoldingID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "invalid holdingId: "+row.HoldingID)
			return
		}
		v := row.CostBasis
		updates = append(updates, db.CostBasisUpdate{ID: id, CostBasis: &v})
	}
	if err := s.holdings.BulkUpdateCostBasis(r.Context(), userID, updates); err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"updated": len(updates)})
}
