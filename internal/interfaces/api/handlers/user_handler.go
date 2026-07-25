package handlers

import (
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type userHandler struct {
	getUser usecases.GetUserUseCase
	log     logger.Logger
}

// NewUserHandler returns interface type for dependency injection.
func NewUserHandler(getUser usecases.GetUserUseCase, log logger.Logger) UserHandler {
	return &userHandler{getUser: getUser, log: log}
}

// Me handles GET /me: the authenticated caller's own profile.
func (h *userHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.getUser.Execute(r.Context(), userID)
	if err != nil {
		writeUsecaseError(h.log, w, "get user", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, toUserResponse(user))
}

func toUserResponse(u *dto.UserDTO) interfacedto.UserResponse {
	return interfacedto.UserResponse{
		ID:               u.ID,
		Email:            u.Email,
		DisplayName:      u.DisplayName,
		CloudSyncEnabled: u.CloudSyncEnabled,
		CreatedAt:        u.CreatedAt,
	}
}
