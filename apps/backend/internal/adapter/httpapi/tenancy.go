package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/usecase"
	"github.com/gin-gonic/gin"
)

func (s *Server) authContexts(c *gin.Context) {
	result, err := s.deps.Tenancy.Contexts(c.Request.Context(), principal(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) switchContext(c *gin.Context) {
	var input domain.SwitchContextInput
	if err := decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	result, err := s.deps.Auth.SwitchContext(c.Request.Context(), authentication(c), input)
	if err != nil {
		writeError(c, err)
		return
	}
	attachDataScope(c, result.Principal)
	data, _ := s.sessionView(c, result.Principal)
	data["sessionToken"] = result.Token
	writeData(c, http.StatusOK, data)
}

func (s *Server) registerInvitation(c *gin.Context) {
	var input domain.RegisterInvitationInput
	if err := decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	if s.deps.Auth.Limiter != nil {
		allowed, err := s.deps.Auth.Limiter.Allow(c.Request.Context(), "invitation-register:"+c.ClientIP(), s.deps.Auth.RateLimit, s.deps.Auth.RateWindow)
		if err != nil {
			writeError(c, domain.WrapInternal(err, "registration limiter"))
			return
		}
		if !allowed {
			writeError(c, domain.NewError(domain.CodeRateLimited, "Terlalu banyak percobaan. Coba lagi nanti"))
			return
		}
	}
	_, err := s.deps.Tenancy.Register(c.Request.Context(), input)
	if err != nil {
		writeError(c, err)
		return
	}
	result, err := s.deps.Auth.Login(c.Request.Context(), domain.LoginInput{Username: input.Username, Password: input.Password, IPAddress: c.ClientIP(), ClientProtocolVersion: 2})
	if err != nil {
		writeError(c, err)
		return
	}
	data, _ := s.sessionView(c, result.Principal)
	data["sessionToken"] = result.Token
	writeData(c, http.StatusCreated, data)
}

func (s *Server) acceptInvitation(c *gin.Context) {
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	if s.deps.Auth.Limiter != nil {
		allowed, err := s.deps.Auth.Limiter.Allow(c.Request.Context(), "invitation-accept:"+principal(c).UserID.String(), s.deps.Auth.RateLimit, s.deps.Auth.RateWindow)
		if err != nil {
			writeError(c, domain.WrapInternal(err, "invitation limiter"))
			return
		}
		if !allowed {
			writeError(c, domain.NewError(domain.CodeRateLimited, "Terlalu banyak percobaan undangan. Coba lagi nanti"))
			return
		}
	}
	result, err := s.deps.Tenancy.Accept(c.Request.Context(), principal(c), input.Code)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) platformTenants(c *gin.Context) {
	if err := usecase.RequirePlatform(principal(c)); err != nil {
		writeError(c, err)
		return
	}
	result, err := s.deps.Tenancy.Repo.ListTenants(c.Request.Context(), principal(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) createTenant(c *gin.Context) {
	var input domain.CreateTenantInput
	if err := decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	result, err := s.deps.Tenancy.Create(c.Request.Context(), principal(c), input)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusCreated, result.Tenant)
}

func (s *Server) setTenantStatus(c *gin.Context) {
	if err := usecase.RequirePlatform(principal(c)); err != nil {
		writeError(c, err)
		return
	}
	id, err := parseUUID(c.Param("tenantId"), "tenantId")
	if err != nil {
		writeError(c, err)
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if err = decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	if input.Status != "active" && input.Status != "suspended" {
		writeError(c, domain.Validation("Status bisnis harus aktif atau dinonaktifkan", nil))
		return
	}
	result, err := s.deps.Tenancy.Repo.SetTenantStatus(c.Request.Context(), principal(c), id, input.Status)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) ownerInvitation(c *gin.Context) {
	id, err := parseUUID(c.Param("tenantId"), "tenantId")
	if err != nil {
		writeError(c, err)
		return
	}
	result, err := s.deps.Tenancy.Invite(c.Request.Context(), principal(c), id, domain.RoleSuperadmin, true)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusCreated, result)
}

func (s *Server) platformAudit(c *gin.Context) {
	if err := usecase.RequirePlatform(principal(c)); err != nil {
		writeError(c, err)
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit < 1 || limit > 200 {
		limit = 50
	}
	result, err := s.deps.Tenancy.Repo.ListPlatformAudit(c.Request.Context(), principal(c), limit)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) tenantMembers(c *gin.Context) {
	if err := usecase.RequireSuperadmin(principal(c)); err != nil {
		writeError(c, err)
		return
	}
	result, err := s.deps.Tenancy.Repo.TenantMembers(c.Request.Context(), principal(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) updateTenantMember(c *gin.Context) {
	if err := usecase.RequireProduction(principal(c)); err != nil {
		writeError(c, err)
		return
	}
	if err := usecase.RequireSuperadmin(principal(c)); err != nil {
		writeError(c, err)
		return
	}
	id, err := parseUUID(c.Param("userId"), "userId")
	if err != nil {
		writeError(c, err)
		return
	}
	var input domain.UpdateMembershipInput
	if err = decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	if input.Role != nil && !input.Role.Valid() || input.Role == nil && input.Active == nil {
		writeError(c, domain.Validation("Perubahan anggota tidak valid", nil))
		return
	}
	result, err := s.deps.Tenancy.Repo.UpdateTenantMember(c.Request.Context(), principal(c), id, input)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) tenantInvitations(c *gin.Context) {
	if err := usecase.RequireSuperadmin(principal(c)); err != nil {
		writeError(c, err)
		return
	}
	result, err := s.deps.Tenancy.Repo.ListTenantInvitations(c.Request.Context(), principal(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) inviteTenantMember(c *gin.Context) {
	var input struct {
		Role domain.Role `json:"role"`
	}
	if err := decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	result, err := s.deps.Tenancy.Invite(c.Request.Context(), principal(c), principal(c).TenantID, input.Role, false)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusCreated, result)
}

func (s *Server) revokeInvitation(c *gin.Context) {
	if err := usecase.RequireProduction(principal(c)); err != nil {
		writeError(c, err)
		return
	}
	if err := usecase.RequireSuperadmin(principal(c)); err != nil {
		writeError(c, err)
		return
	}
	id, err := parseUUID(c.Param("invitationId"), "invitationId")
	if err != nil {
		writeError(c, err)
		return
	}
	if err = s.deps.Tenancy.Repo.RevokeTenantInvitation(c.Request.Context(), principal(c), id); err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, gin.H{"success": true})
}

func (s *Server) tenantProfile(c *gin.Context) {
	result, err := s.deps.Tenancy.Repo.GetTenantProfile(c.Request.Context(), principal(c).TenantID)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}
func (s *Server) updateTenantProfile(c *gin.Context) {
	var input domain.UpdateTenantProfileInput
	if err := decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	result, err := s.deps.Tenancy.UpdateProfile(c.Request.Context(), principal(c), input)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}
func (s *Server) tenantQRIS(c *gin.Context) {
	result, err := s.deps.Tenancy.Repo.GetTenantQRIS(c.Request.Context(), principal(c).TenantID)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}
func (s *Server) updateTenantQRIS(c *gin.Context) {
	var input domain.UpdateTenantQRISInput
	if err := decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	result, err := s.deps.Tenancy.UpdateQRIS(c.Request.Context(), principal(c), input)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}
func (s *Server) updateOwnProfile(c *gin.Context) {
	p := principal(c)
	if err := usecase.RequireReady(p); err != nil {
		writeError(c, err)
		return
	}
	if p.ContextKind == domain.ContextTenant && p.DataMode == domain.DataModeSandbox {
		writeError(c, domain.NewError(domain.CodeForbidden, "Ubah profil melalui akun atau mode produksi"))
		return
	}
	var input struct {
		FullName string `json:"fullName"`
	}
	if err := decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	input.FullName = strings.TrimSpace(input.FullName)
	if input.FullName == "" || len(input.FullName) > 160 {
		writeError(c, domain.Validation("Nama lengkap tidak valid", nil))
		return
	}
	result, err := s.deps.Tenancy.Repo.UpdateOwnProfile(c.Request.Context(), p, input.FullName)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) revalidateOrigins(c *gin.Context) {
	var input struct {
		Origins []domain.OutboxOrigin `json:"origins"`
	}
	if err := decodeJSON(c, &input); err != nil {
		writeError(c, err)
		return
	}
	if len(input.Origins) > 100 {
		writeError(c, domain.Validation("Maksimal 100 identitas per pemeriksaan", nil))
		return
	}
	result, err := s.deps.Tenancy.Repo.RevalidateOrigins(c.Request.Context(), principal(c), input.Origins)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, gin.H{"origins": result})
}
