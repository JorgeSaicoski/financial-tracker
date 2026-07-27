package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type planHandler struct {
	createPlan usecases.CreatePlanUseCase
	listPlans  usecases.ListPlansUseCase
	getPlan    usecases.GetPlanUseCase
	updatePlan usecases.UpdatePlanUseCase

	log logger.Logger
}

// NewPlanHandler returns interface type for dependency injection.
func NewPlanHandler(
	createPlan usecases.CreatePlanUseCase,
	listPlans usecases.ListPlansUseCase,
	getPlan usecases.GetPlanUseCase,
	updatePlan usecases.UpdatePlanUseCase,
	log logger.Logger,
) PlanHandler {
	return &planHandler{
		createPlan: createPlan,
		listPlans:  listPlans,
		getPlan:    getPlan,
		updatePlan: updatePlan,
		log:        log,
	}
}

// CreatePlan handles POST /plans.
func (h *planHandler) CreatePlan(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.CreatePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	var startDate time.Time
	if req.StartDate != nil {
		startDate = *req.StartDate
	}

	plan, err := h.createPlan.Execute(r.Context(), usecases.CreatePlanInput{
		UserID:              userID,
		Name:                req.Name,
		Type:                req.Type,
		TargetAmount:        req.TargetAmount,
		Currency:            req.Currency,
		MonthlyTargetAmount: req.MonthlyTargetAmount,
		AccountID:           req.AccountID,
		StartDate:           startDate,
		EndDate:             req.EndDate,
	})
	if err != nil {
		h.writeUsecaseError(w, "create plan", err)
		return
	}
	writeJSON(h.log, w, http.StatusCreated, toPlanResponse(plan))
}

// ListPlans handles GET /plans: every plan the caller owns, each with
// lightweight progress (see usecases.PlanSummary's own doc comment for
// what "lightweight" omits and why).
func (h *planHandler) ListPlans(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	summaries, err := h.listPlans.Execute(r.Context(), userID, time.Now().UTC())
	if err != nil {
		h.writeUsecaseError(w, "list plans", err)
		return
	}
	resp := interfacedto.ListPlansResponse{Plans: make([]interfacedto.PlanSummaryResponse, 0, len(summaries))}
	for _, s := range summaries {
		resp.Plans = append(resp.Plans, toPlanSummaryResponse(s))
	}
	writeJSON(h.log, w, http.StatusOK, resp)
}

// GetPlan handles GET /plans/{id}: full progress plus the pace checker
// (on_track, and — for a savings plan — projected_shortfall).
func (h *planHandler) GetPlan(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	detail, err := h.getPlan.Execute(r.Context(), userID, r.PathValue("id"), time.Now().UTC())
	if err != nil {
		h.writeUsecaseError(w, "get plan", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, interfacedto.PlanDetailResponse{
		PlanSummaryResponse: toPlanSummaryResponse(detail.PlanSummary),
		OnTrack:             detail.OnTrack,
		ProjectedShortfall:  detail.ProjectedShortfall,
	})
}

// UpdatePlan handles PATCH /plans/{id}: edit amounts/dates, or change
// status — status never changes as a side effect of a GET.
func (h *planHandler) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.UpdatePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	plan, err := h.updatePlan.Execute(r.Context(), userID, r.PathValue("id"), usecases.UpdatePlanInput{
		Name:                req.Name,
		TargetAmount:        req.TargetAmount,
		MonthlyTargetAmount: req.MonthlyTargetAmount,
		EndDate:             req.EndDate,
		Status:              req.Status,
	})
	if err != nil {
		h.writeUsecaseError(w, "update plan", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, toPlanResponse(plan))
}

func toPlanResponse(p *dto.PlanDTO) interfacedto.PlanResponse {
	return interfacedto.PlanResponse{
		ID:                  p.ID,
		Name:                p.Name,
		Type:                p.Type,
		TargetAmount:        p.TargetAmount,
		Currency:            p.Currency,
		MonthlyTargetAmount: p.MonthlyTargetAmount,
		AccountID:           p.AccountID,
		StartDate:           p.StartDate,
		EndDate:             p.EndDate,
		Status:              p.Status,
		CreatedAt:           p.CreatedAt,
	}
}

func toPlanSummaryResponse(s usecases.PlanSummary) interfacedto.PlanSummaryResponse {
	return interfacedto.PlanSummaryResponse{
		Plan:          toPlanResponse(s.Plan),
		Progress:      s.Progress,
		TargetReached: s.TargetReached,
	}
}

func (h *planHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	switch {
	case apperrors.Is(err, apperrors.ErrInvalidInput):
		writeError(h.log, w, http.StatusBadRequest, err.Error())
	case apperrors.Is(err, apperrors.ErrNotFound):
		writeError(h.log, w, http.StatusNotFound, "plan not found")
	case apperrors.Is(err, apperrors.ErrConflict):
		writeError(h.log, w, http.StatusConflict, err.Error())
	default:
		h.log.Error("%s failed: %v", action, err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
	}
}
