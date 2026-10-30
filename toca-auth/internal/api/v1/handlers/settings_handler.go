package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/oginoino/toca-auth/internal/domain/user"
	"github.com/oginoino/toca-auth/internal/service"
	appcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/validation"
)

type SettingsHandler struct {
	authService     *service.AuthService
	prefsService    *service.PreferencesService
	passwordService *service.PasswordService
}

func NewSettingsHandler(
	authService *service.AuthService,
	prefsService *service.PreferencesService,
	passwordService *service.PasswordService,
) *SettingsHandler {
	return &SettingsHandler{
		authService:     authService,
		prefsService:    prefsService,
		passwordService: passwordService,
	}
}

func (h *SettingsHandler) UpdateProfile(c *gin.Context) {
	accountID := c.GetString("user_id")
	if accountID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req user.User
	if !validation.BindJSON(c, &req) {
		return
	}

	if err := h.authService.UpdateProfile(c.Request.Context(), accountID, &req); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "profile updated successfully", nil)
}

type changePasswordReq struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

func (h *SettingsHandler) ChangePassword(c *gin.Context) {
	accountID := c.GetString("user_id")
	if accountID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req changePasswordReq
	if !validation.BindJSON(c, &req) {
		return
	}

	// P1/A14: o access token do chamador é necessário para REVOGAR as sessões
	// depois da troca (o Supabase espera o token, não o account id).
	accessToken, _ := appcontext.GetAuthToken(c.Request.Context())
	if accessToken == "" {
		accessToken = c.GetString("auth_token")
	}

	if err := h.passwordService.ChangePassword(c.Request.Context(), accessToken, accountID, req.CurrentPassword, req.NewPassword); err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	response.Success(c, "password updated successfully", nil)
}

func (h *SettingsHandler) GetPreferences(c *gin.Context) {
	accountID := c.GetString("user_id")
	if accountID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	prefs, err := h.prefsService.GetPreferences(c.Request.Context(), accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, prefs)
}

func (h *SettingsHandler) UpdatePreferences(c *gin.Context) {
	accountID := c.GetString("user_id")
	if accountID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.ValidationError(c, "invalid request body")
		return
	}

	var patch map[string]json.RawMessage
	if len(body) > 0 {
		if err := json.Unmarshal(body, &patch); err != nil {
			response.ValidationError(c, err.Error())
			return
		}
	}

	if err := h.prefsService.PatchPreferences(c.Request.Context(), accountID, patch); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "preferences updated successfully", nil)
}

func (h *SettingsHandler) UploadAvatar(c *gin.Context) {
	accountID := c.GetString("user_id")
	if accountID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	file, err := c.FormFile("avatar")
	if err != nil {
		response.ValidationError(c, "avatar file is required")
		return
	}

	src, err := file.Open()
	if err != nil {
		response.InternalServerError(c, "failed to open file")
		return
	}
	defer src.Close()

	head := make([]byte, 512)
	n, err := src.Read(head)
	if err != nil && err != io.EOF {
		response.InternalServerError(c, "failed to read file header")
		return
	}

	ext, detectedMIME, err := storage.ValidateWithMIME("toca-avatars", file.Filename, file.Size, head[:n])
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	multiSrc := io.MultiReader(bytes.NewReader(head[:n]), src)

	uniqueFilename := fmt.Sprintf("%s%s", uuid.NewString(), ext)

	avatarURL, err := h.authService.UploadAvatar(c.Request.Context(), accountID, uniqueFilename, detectedMIME, multiSrc)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Avatar uploaded successfully", gin.H{"avatar_url": avatarURL})
}
