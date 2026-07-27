package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type categoryHandler struct {
	createCategory usecases.CreateCategoryUseCase
	updateCategory usecases.UpdateCategoryUseCase
	deleteCategory usecases.DeleteCategoryUseCase

	log logger.Logger
}

// NewCategoryHandler returns interface type for dependency injection.
func NewCategoryHandler(
	createCategory usecases.CreateCategoryUseCase,
	updateCategory usecases.UpdateCategoryUseCase,
	deleteCategory usecases.DeleteCategoryUseCase,
	log logger.Logger,
) CategoryHandler {
	return &categoryHandler{
		createCategory: createCategory,
		updateCategory: updateCategory,
		deleteCategory: deleteCategory,
		log:            log,
	}
}

// CreateCategory handles POST /categories.
func (h *categoryHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.CreateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	category, err := h.createCategory.Execute(r.Context(), usecases.CreateCategoryInput{
		UserID:              userID,
		Name:                req.Name,
		AvoidabilityPercent: req.AvoidabilityPercent,
	})
	if err != nil {
		h.writeUsecaseError(w, "create category", err)
		return
	}
	writeJSON(h.log, w, http.StatusCreated, toCategoryResponse(category))
}

// UpdateCategory handles PATCH /categories/{id}.
func (h *categoryHandler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.UpdateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	category, err := h.updateCategory.Execute(r.Context(), userID, r.PathValue("id"), usecases.UpdateCategoryInput{
		Name:                req.Name,
		AvoidabilityPercent: req.AvoidabilityPercent,
		IsDefault:           req.IsDefault,
	})
	if err != nil {
		h.writeUsecaseError(w, "update category", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, toCategoryResponse(category))
}

// DeleteCategory handles DELETE /categories/{id}. category_id is a real
// foreign key (BACK-14 follow-up): any movement/purchase referencing id
// is reassigned to the caller's default category first, so deletion
// still always succeeds unless id itself is the default (see
// DeleteCategoryUseCase).
func (h *categoryHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.deleteCategory.Execute(r.Context(), userID, r.PathValue("id")); err != nil {
		h.writeUsecaseError(w, "delete category", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toCategoryResponse(c *dto.CategoryDTO) interfacedto.CategoryResponse {
	return interfacedto.CategoryResponse{
		ID:                  c.ID,
		Name:                c.Name,
		AvoidabilityPercent: c.AvoidabilityPercent,
		IsDefault:           c.IsDefault,
	}
}

func (h *categoryHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	switch {
	case apperrors.Is(err, apperrors.ErrInvalidInput):
		writeError(h.log, w, http.StatusBadRequest, err.Error())
	case apperrors.Is(err, apperrors.ErrNotFound):
		writeError(h.log, w, http.StatusNotFound, "category not found")
	case apperrors.Is(err, apperrors.ErrConflict):
		writeError(h.log, w, http.StatusConflict, err.Error())
	default:
		h.log.Error("%s failed: %v", action, err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
	}
}
