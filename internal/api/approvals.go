// Package api: HITL approvals endpoint handlers (SPEC-015 §3.5).
//
// These handlers delegate to the HITL Manager (hitl.Manager) for all approval
// lifecycle operations. Raw SQL is no longer used — the Manager is the single
// authority for approval state transitions, expiry, and notifications.
//
// axiom:trace work_item=spec-014-hardening-01 spec=specs/014-hitl-interrupt-state.md plan=phase-1/task-1-3/step-1-3-1 impl=internal/api/approvals.go
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/wojons/conscientiousness/internal/hitl"
)

// ============================================================================
// GET /api/v1/approvals — List pending approvals
// ============================================================================

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	if GetAuthScope(r) != "admin" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "admin scope required")
		return
	}

	sessionFilter := r.URL.Query().Get("session_id")
	ctx := r.Context()

	// Use HITL Manager if available
	if s.hitl != nil {
		if sessionFilter != "" {
			reqs, err := s.hitl.ListPendingForSession(ctx, sessionFilter)
			if err != nil {
				writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list session approvals")
				return
			}
			writeJSON(w, hitlRequestsToResponses(reqs))
			return
		}
		reqs, err := s.hitl.ListPendingApprovals(ctx)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list approvals")
			return
		}
		writeJSON(w, hitlRequestsToResponses(reqs))
		return
	}

	// Fallback: raw SQL (legacy path for tests without HITL Manager)
	writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "HITL Manager not available")
}

// ============================================================================
// GET /api/v1/approvals/:id — Get approval details
// ============================================================================

func (s *Server) handleGetApproval(w http.ResponseWriter, r *http.Request, approvalID string) {
	if GetAuthScope(r) != "admin" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "admin scope required")
		return
	}

	ctx := r.Context()
	if s.hitl != nil {
		req, err := s.hitl.GetApproval(ctx, approvalID)
		if err != nil {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "approval not found")
			return
		}
		writeJSON(w, hitlRequestToResponse(req))
		return
	}
	writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "HITL Manager not available")
}

// ============================================================================
// POST /api/v1/approvals/:id/review — Approve/reject/modify
// ============================================================================

func (s *Server) handleReviewApproval(w http.ResponseWriter, r *http.Request, approvalID string) {
	if GetAuthScope(r) != "admin" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "admin scope required")
		return
	}

	var req ApprovalReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	if req.Decision != "approved" && req.Decision != "rejected" && req.Decision != "modified" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "decision must be: approved, rejected, or modified")
		return
	}

	if req.Decision == "modified" && req.ModifiedSQL == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "modified_sql required when decision is 'modified'")
		return
	}

	ctx := r.Context()
	reviewerID := GetAuthSessionID(r)
	if reviewerID == "" {
		reviewerID = "admin"
	}

	// Use HITL Manager for decision processing
	if s.hitl != nil {
		decision := hitl.Decision(req.Decision)
		err := s.hitl.ReviewApproval(ctx, approvalID, decision, reviewerID, req.Notes, req.ModifiedSQL)
		if err != nil {
			if strings.Contains(err.Error(), "not pending") {
				writeError(w, r, http.StatusConflict, "CONFLICT", "approval is not pending, cannot be reviewed")
				return
			}
			if strings.Contains(err.Error(), "not found") {
				writeError(w, r, http.StatusNotFound, "NOT_FOUND", "approval not found")
				return
			}
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}

		// Fetch and return updated record
		updated, err := s.hitl.GetApproval(ctx, approvalID)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch updated approval")
			return
		}
		writeJSON(w, hitlRequestToResponse(updated))
		return
	}

	writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "HITL Manager not available")
}

// ============================================================================
// GET /api/v1/sessions/:id/approvals — List approvals for session
// ============================================================================

func (s *Server) handleSessionApprovals(w http.ResponseWriter, r *http.Request, sessionID string) {
	if !s.checkSessionAccess(w, r, sessionID) {
		return
	}

	scope := GetAuthScope(r)
	if scope != "admin" && scope != "session" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "admin or session scope required")
		return
	}

	statusFilter := r.URL.Query().Get("status")
	ctx := r.Context()

	if s.hitl != nil {
		// Support status=all and status=<any specific status>
		if statusFilter == "all" {
			reqs, err := s.hitl.ListSessionApprovals(ctx, sessionID)
			if err != nil {
				writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list session approvals")
				return
			}
			writeJSON(w, hitlRequestsToResponses(reqs))
			return
		}
		if statusFilter != "" && statusFilter != "pending" {
			reqs, err := s.hitl.ListSessionApprovalsByStatus(ctx, sessionID, hitl.ApprovalStatus(statusFilter))
			if err != nil {
				writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list session approvals")
				return
			}
			writeJSON(w, hitlRequestsToResponses(reqs))
			return
		}
		// Default: status=pending (or empty falls through to pending)
		reqs, err := s.hitl.ListPendingForSession(ctx, sessionID)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list session approvals")
			return
		}
		writeJSON(w, hitlRequestsToResponses(reqs))
		return
	}
	writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "HITL Manager not available")
}

// ============================================================================
// HITL Request ⇄ API Response Conversion
// ============================================================================

func hitlRequestToResponse(req *hitl.ApprovalRequest) ApprovalResponse {
	r := ApprovalResponse{
		ID:          req.ID,
		SessionID:   req.SessionID,
		Iteration:   req.Iteration,
		RequestType: string(req.RequestType),
		Description: req.Description,
		RiskLevel:   string(req.RiskLevel),
		Context:     parseJSONRaw(req.Context),
		Status:      string(req.Status),
		CreatedAt:   req.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if req.TargetTool != "" {
		r.TargetTool = &req.TargetTool
	}
	if req.TargetSQL != "" {
		r.TargetSQL = &req.TargetSQL
	}
	if req.ReviewerID != "" {
		r.ReviewerID = &req.ReviewerID
	}
	if req.ReviewNotes != "" {
		r.ReviewNotes = &req.ReviewNotes
	}
	if req.ModifiedSQL != "" {
		r.ModifiedSQL = &req.ModifiedSQL
	}
	if req.ReviewedAt != nil {
		s := req.ReviewedAt.Format("2006-01-02T15:04:05Z")
		r.ReviewedAt = &s
	}
	if !req.ExpiresAt.IsZero() {
		s := req.ExpiresAt.Format("2006-01-02T15:04:05Z")
		r.ExpiresAt = &s
	}
	return r
}

func hitlRequestsToResponses(reqs []hitl.ApprovalRequest) []ApprovalResponse {
	result := make([]ApprovalResponse, 0, len(reqs))
	for i := range reqs {
		result = append(result, hitlRequestToResponse(&reqs[i]))
	}
	return result
}
