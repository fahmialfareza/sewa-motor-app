package httpapi

import (
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/usecase"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (s *Server) managementService(c *gin.Context) (usecase.Management, bool) {
	repo, ok := s.deps.Tenancy.Repo.(port.ManagementRepository)
	if !ok {
		writeError(c, domain.NewError(domain.CodeClientUpdateRequired, "Backend belum mendukung pengelolaan organisasi"))
		return usecase.Management{}, false
	}
	return usecase.Management{Repo: repo, Passwords: s.deps.Tenancy.Passwords}, true
}

func (s *Server) invitationsRemoved(c *gin.Context) {
	writeError(c, domain.NewError("INVITATIONS_REMOVED", "Undangan bisnis tidak lagi digunakan. Semua akun aktif dapat memilih setiap bisnis. Hubungi Superadmin untuk membuat akun."))
}

func (s *Server) upgradeSession(c *gin.Context) {
	var input struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err := decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	result, err := s.deps.Auth.UpgradeSession(c.Request.Context(), authentication(c), input.ProtocolVersion)
	if err != nil {
		writeError(c, err)
		return
	}
	attachDataScope(c, result.Principal)
	data, err := s.sessionView(c, result.Principal)
	if err != nil {
		writeError(c, err)
		return
	}
	data["sessionToken"] = result.Token
	writeData(c, http.StatusOK, data)
}

func (s *Server) managedUsers(c *gin.Context) {
	service, ok := s.managementService(c)
	if !ok {
		return
	}
	result, err := service.ListUsers(c.Request.Context(), principal(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) managedUser(c *gin.Context) {
	service, ok := s.managementService(c)
	if !ok {
		return
	}
	id, err := parseUUID(c.Param("userId"), "userId")
	if err != nil {
		writeError(c, err)
		return
	}
	result, err := service.GetUser(c.Request.Context(), principal(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) createManagedUser(c *gin.Context) {
	service, ok := s.managementService(c)
	if !ok {
		return
	}
	var body struct {
		FullName          string      `json:"fullName"`
		Username          string      `json:"username"`
		Role              domain.Role `json:"role"`
		TemporaryPassword string      `json:"temporaryPassword"`
	}
	if err := decodeJSON(c, &body); err != nil {
		writeError(c, err)
		return
	}
	result, err := service.CreateUser(c.Request.Context(), principal(c), domain.CreateUserInput{FullName: body.FullName, Username: body.Username, Role: body.Role, TemporaryPassword: body.TemporaryPassword})
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusCreated, result)
}

func (s *Server) updateManagedUser(c *gin.Context) {
	service, ok := s.managementService(c)
	if !ok {
		return
	}
	id, err := parseUUID(c.Param("userId"), "userId")
	if err != nil {
		writeError(c, err)
		return
	}
	var input domain.UpdateManagedUserInput
	if err = decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	result, err := service.UpdateUser(c.Request.Context(), principal(c), id, input)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) resetManagedPassword(c *gin.Context) {
	service, ok := s.managementService(c)
	if !ok {
		return
	}
	id, err := parseUUID(c.Param("userId"), "userId")
	if err != nil {
		writeError(c, err)
		return
	}
	var input struct {
		TemporaryPassword string `json:"temporaryPassword"`
	}
	if err = decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	result, err := service.ResetPassword(c.Request.Context(), principal(c), id, input.TemporaryPassword)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) updateManagedTenant(c *gin.Context) {
	service, ok := s.managementService(c)
	if !ok {
		return
	}
	id, err := parseUUID(c.Param("tenantId"), "tenantId")
	if err != nil {
		writeError(c, err)
		return
	}
	var input domain.UpdateTenantInput
	if err = decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	result, err := service.UpdateTenant(c.Request.Context(), principal(c), id, input)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}
