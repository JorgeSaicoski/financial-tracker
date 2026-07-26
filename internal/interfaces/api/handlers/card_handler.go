package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type cardHandler struct {
	createCard usecases.CreateCardUseCase
	listCards  usecases.ListCardsUseCase
	getCard    usecases.GetCardUseCase
	updateCard usecases.UpdateCardUseCase
	deleteCard usecases.DeleteCardUseCase

	log logger.Logger
}

// NewCardHandler returns interface type for dependency injection.
func NewCardHandler(
	createCard usecases.CreateCardUseCase,
	listCards usecases.ListCardsUseCase,
	getCard usecases.GetCardUseCase,
	updateCard usecases.UpdateCardUseCase,
	deleteCard usecases.DeleteCardUseCase,
	log logger.Logger,
) CardHandler {
	return &cardHandler{
		createCard: createCard,
		listCards:  listCards,
		getCard:    getCard,
		updateCard: updateCard,
		deleteCard: deleteCard,
		log:        log,
	}
}

// CreateCard handles POST /cards.
func (h *cardHandler) CreateCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.CreateCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	card, err := h.createCard.Execute(r.Context(), usecases.CreateCardInput{
		UserID:        userID,
		Name:          req.Name,
		LastFour:      req.LastFour,
		ClosingDay:    req.ClosingDay,
		DueDay:        req.DueDay,
		CreditLimit:   req.CreditLimit,
		MonthlyBudget: req.MonthlyBudget,
		Currency:      req.Currency,
	})
	if err != nil {
		h.writeUsecaseError(w, "create card", err)
		return
	}

	// A brand-new card has no movements: its computed view is all zeros
	// / today's due date, no lookups needed.
	view, err := h.getCard.Execute(r.Context(), userID, card.ID)
	if err != nil {
		h.writeUsecaseError(w, "create card", err)
		return
	}
	writeJSON(h.log, w, http.StatusCreated, toCardResponse(view))
}

// ListCards handles GET /cards: every card with its computed amount-due/
// limit/budget picture, scoped to the authenticated user.
func (h *cardHandler) ListCards(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	views, err := h.listCards.Execute(r.Context(), userID)
	if err != nil {
		h.writeUsecaseError(w, "list cards", err)
		return
	}

	resp := interfacedto.CardsResponse{Cards: make([]interfacedto.CardResponse, 0, len(views))}
	for _, v := range views {
		resp.Cards = append(resp.Cards, toCardResponse(v))
	}
	writeJSON(h.log, w, http.StatusOK, resp)
}

// GetCard handles GET /cards/{id}.
func (h *cardHandler) GetCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	view, err := h.getCard.Execute(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		h.writeUsecaseError(w, "get card", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, toCardResponse(view))
}

// UpdateCard handles PATCH /cards/{id}.
func (h *cardHandler) UpdateCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.UpdateCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	_, err := h.updateCard.Execute(r.Context(), userID, r.PathValue("id"), usecases.UpdateCardInput{
		Name:          req.Name,
		LastFour:      req.LastFour,
		ClosingDay:    req.ClosingDay,
		DueDay:        req.DueDay,
		CreditLimit:   req.CreditLimit,
		MonthlyBudget: req.MonthlyBudget,
	})
	if err != nil {
		h.writeUsecaseError(w, "update card", err)
		return
	}

	view, err := h.getCard.Execute(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		h.writeUsecaseError(w, "update card", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, toCardResponse(view))
}

// DeleteCard handles DELETE /cards/{id}. Rejected (409) if any movement
// or credit-card purchase still references it.
func (h *cardHandler) DeleteCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.deleteCard.Execute(r.Context(), userID, r.PathValue("id")); err != nil {
		h.writeUsecaseError(w, "delete card", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toCardResponse(v usecases.CardView) interfacedto.CardResponse {
	return interfacedto.CardResponse{
		ID:              v.Card.ID,
		UserID:          v.Card.UserID,
		Name:            v.Card.Name,
		LastFour:        v.Card.LastFour,
		ClosingDay:      v.Card.ClosingDay,
		DueDay:          v.Card.DueDay,
		CreditLimit:     v.Card.CreditLimit,
		MonthlyBudget:   v.Card.MonthlyBudget,
		Currency:        v.Card.Currency,
		CreatedAt:       v.Card.CreatedAt,
		NextDueTotal:    v.NextDueTotal,
		NextDueDate:     v.NextDueDate,
		OpenCycleTotal:  v.OpenCycleTotal,
		AvailableCredit: v.AvailableCredit,
		BudgetRemaining: v.BudgetRemaining,
		OverBudget:      v.OverBudget,
	}
}

func (h *cardHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	switch {
	case apperrors.Is(err, apperrors.ErrInvalidInput):
		writeError(h.log, w, http.StatusBadRequest, err.Error())
	case apperrors.Is(err, apperrors.ErrNotFound):
		writeError(h.log, w, http.StatusNotFound, "card not found")
	case apperrors.Is(err, apperrors.ErrConflict):
		writeError(h.log, w, http.StatusConflict, err.Error())
	default:
		h.log.Error("%s failed: %v", action, err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
	}
}
