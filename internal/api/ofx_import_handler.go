package api

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	ofxsvc "github.com/cybirdsecurity/myfi-os/internal/services/ofx"
)

// handleOFXPreview parses an uploaded OFX file and returns a per-account diff
// without writing anything to the database.
//
// POST /import/ofx/preview  (multipart/form-data, field "file")
func (s *Server) handleOFXPreview(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "could not parse multipart form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "file field is required")
		return
	}
	defer file.Close()

	// Buffer the file so we can pass it to Apply later if needed.
	// The preview response includes enough data for the frontend to re-send accepted rows.
	result, err := s.ofxImporter.Preview(r.Context(), userID, file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "OFX_PARSE_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleOFXApply accepts the user's mapping + accepted rows and writes the
// holdings replacement to the database.
//
// POST /import/ofx/apply  (application/json)
//
// Body:
//
//	{
//	  "rawOfx":   "<OFX>...",           // the raw OFX text re-submitted by the client
//	  "mappings": [{ "ofxAccountId": "12345", "myfiAccountId": "uuid" }],
//	  "accepted": [{ "ofxAccountId": "12345", "ticker": "VTSAX" }]
//	}
func (s *Server) handleOFXApply(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	var body struct {
		RawOFX   string                 `json:"rawOfx"`
		Filename string                 `json:"filename"`
		Mappings []ofxsvc.ApplyMapping  `json:"mappings"`
		Accepted []ofxsvc.ApplyAccepted `json:"accepted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.RawOFX == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION", "rawOfx is required")
		return
	}
	if len(body.Mappings) == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "at least one account mapping is required")
		return
	}

	// Re-parse the OFX from the raw text the client submitted.
	stmts, err := ofxsvc.ParseOFX(bytes.NewReader([]byte(body.RawOFX)))
	if err != nil {
		writeError(w, http.StatusBadRequest, "OFX_PARSE_ERROR", err.Error())
		return
	}

	result, err := s.ofxImporter.Apply(r.Context(), userID, stmts, body.Mappings, body.Accepted)
	if err != nil {
		_ = s.importLogs.Create(r.Context(), db.CreateImportLogParams{
			UserID:     userID,
			ImportType: "ofx_holdings",
			Filename:   body.Filename,
			Status:     "failed",
		})
		writeError(w, http.StatusInternalServerError, "APPLY_ERROR", err.Error())
		return
	}

	status := "success"
	if result.Replaced == 0 {
		status = "failed"
	}
	_ = s.importLogs.Create(r.Context(), db.CreateImportLogParams{
		UserID:     userID,
		ImportType: "ofx_holdings",
		Filename:   body.Filename,
		Status:     status,
		RowCount:   result.Replaced,
		Skipped:    result.Skipped,
	})

	writeJSON(w, http.StatusOK, result)
}
