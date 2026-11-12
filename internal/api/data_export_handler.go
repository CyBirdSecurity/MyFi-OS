package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/services"
)

func (s *Server) handleImportFull(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	if err := r.ParseMultipartForm(50 << 20); err != nil { // 50 MB
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "could not parse multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "file field is required")
		return
	}
	defer file.Close()

	var bundle services.ExportBundle
	if err := json.NewDecoder(file).Decode(&bundle); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_FILE", "file is not a valid MyFi backup")
		return
	}
	if bundle.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_FILE", "unrecognized backup format — is this a MyFi backup file?")
		return
	}

	summary, err := s.dataPort.Import(r.Context(), userID, &bundle)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "IMPORT_ERROR", err.Error())
		return
	}

	status := "success"
	if len(summary.Warnings) > 0 {
		status = "partial"
	}
	_ = s.importLogs.Create(r.Context(), db.CreateImportLogParams{
		UserID:     userID,
		ImportType: "full_backup",
		Filename:   header.Filename,
		Status:     status,
		RowCount:   summary.AccountsCreated + summary.AccountsMatched,
		Skipped:    0,
		ErrorCount: len(summary.Warnings),
	})

	writeJSON(w, http.StatusOK, summary)
}

// handleExportJSON is an alias used when the caller wants inline JSON (no attachment).
// The download UI in the browser handles the file save via a Blob URL instead.
func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	bundle, err := s.dataPort.Export(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EXPORT_ERROR", err.Error())
		return
	}

	_ = s.importLogs.Create(r.Context(), db.CreateImportLogParams{
		UserID:     userID,
		ImportType: "full_backup",
		Filename:   fmt.Sprintf("myfi-backup-%s.json", time.Now().Format("2006-01-02")),
		Status:     "success",
		RowCount:   len(bundle.Accounts),
	})

	writeJSON(w, http.StatusOK, bundle)
}
