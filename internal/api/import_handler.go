package api

import (
	"net/http"

	"github.com/cybirdsecurity/myfi-os/internal/db"
)

func (s *Server) handleImportCSV(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "could not parse multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "file field is required")
		return
	}
	defer file.Close()

	result, err := s.importer.ImportCSV(r.Context(), userID, file)
	if err != nil {
		_ = s.importLogs.Create(r.Context(), db.CreateImportLogParams{
			UserID:     userID,
			ImportType: "csv_snapshot",
			Filename:   header.Filename,
			Status:     "failed",
		})
		writeError(w, http.StatusBadRequest, "IMPORT_ERROR", err.Error())
		return
	}

	status := "success"
	if result.Imported == 0 {
		status = "failed"
	} else if result.Skipped > 0 || len(result.Errors) > 0 {
		status = "partial"
	}
	_ = s.importLogs.Create(r.Context(), db.CreateImportLogParams{
		UserID:     userID,
		ImportType: "csv_snapshot",
		Filename:   header.Filename,
		Status:     status,
		RowCount:   result.Imported,
		Skipped:    result.Skipped,
		ErrorCount: len(result.Errors),
	})

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleImportHistory(w http.ResponseWriter, r *http.Request) {
	logs, err := s.importLogs.List(r.Context(), userIDFromCtx(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, logs)
}
