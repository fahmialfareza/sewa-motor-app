package httpapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *Server) login(c *gin.Context) {
	var request struct {
		Username       string     `json:"username"`
		Password       string     `json:"password"`
		InstallationID *uuid.UUID `json:"installationId"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	result, err := s.deps.Auth.Login(c.Request.Context(), domain.LoginInput{
		Username: request.Username, Password: request.Password,
		InstallationID: request.InstallationID, IPAddress: c.ClientIP(),
	})
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

func (s *Server) profile(c *gin.Context) {
	current := principal(c)
	data, err := s.sessionView(c, current)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, data)
}

func (s *Server) changePassword(c *gin.Context) {
	var request struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	current := principal(c)
	if err := s.deps.Auth.ChangePassword(c.Request.Context(), current, request.CurrentPassword, request.NewPassword); err != nil {
		writeError(c, err)
		return
	}
	current.MustChangePassword = false
	data, err := s.sessionView(c, current)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, data)
}

func (s *Server) switchMode(c *gin.Context) {
	var request domain.SwitchModeInput
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	auth := authentication(c)
	current := auth.Principal
	// Resolve shared profile/terminal data before rotating the session. Once
	// SwitchMode commits, the old token is revoked and the raw replacement token
	// exists only in this response; no fallible enrichment may happen afterward.
	data, err := s.sessionView(c, current)
	if err != nil {
		writeError(c, err)
		return
	}
	result, err := s.deps.Auth.SwitchMode(c.Request.Context(), auth, request.Mode)
	if err != nil {
		writeError(c, err)
		return
	}
	attachDataScope(c, result.Principal)
	data["sessionId"] = result.Principal.SessionID
	data["dataMode"] = result.Principal.EffectiveDataMode()
	data["dataSpaceId"] = result.Principal.EffectiveDataSpaceID()
	data["sandboxGeneration"] = result.Principal.SandboxGeneration
	data["sessionToken"] = result.Token
	writeData(c, http.StatusOK, data)
}

func (s *Server) sessionView(c *gin.Context, current domain.Principal) (gin.H, error) {
	user, err := s.deps.Repo.GetUser(c.Request.Context(), current.UserID)
	if err != nil {
		return nil, err
	}
	var terminal any
	if current.TerminalID != nil {
		item, terminalErr := s.deps.Repo.GetTerminal(c.Request.Context(), *current.TerminalID)
		if terminalErr != nil {
			return nil, terminalErr
		}
		terminal = item
	}
	return gin.H{
		"sessionId":         current.SessionID,
		"user":              user,
		"terminal":          terminal,
		"dataMode":          current.EffectiveDataMode(),
		"dataSpaceId":       current.EffectiveDataSpaceID(),
		"sandboxGeneration": current.SandboxGeneration,
	}, nil
}

func (s *Server) logout(c *gin.Context) {
	if err := s.deps.Auth.Logout(c.Request.Context(), principal(c)); err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, gin.H{"success": true})
}

func (s *Server) listUsers(c *gin.Context) {
	includeDeleted := c.Query("includeDeleted") == "true"
	users, err := s.deps.Users.List(c.Request.Context(), principal(c), includeDeleted)
	if err != nil {
		writeError(c, err)
		return
	}
	search := strings.ToLower(strings.TrimSpace(c.Query("search")))
	role := domain.Role(c.Query("role"))
	active := c.Query("active")
	filtered := users[:0]
	for _, user := range users {
		if search != "" && !strings.Contains(strings.ToLower(user.FullName+" "+user.Username), search) {
			continue
		}
		if role != "" && user.Role != role {
			continue
		}
		if active == "true" && !user.IsActive || active == "false" && user.IsActive {
			continue
		}
		filtered = append(filtered, user)
	}
	writePage(c, filtered, "")
}

func (s *Server) getUser(c *gin.Context) {
	id, err := parseUUID(c.Param("userId"), "userId")
	if err != nil {
		writeError(c, err)
		return
	}
	user, err := s.deps.Users.Get(c.Request.Context(), principal(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, user)
}

func (s *Server) createUser(c *gin.Context) {
	var request struct {
		FullName          string      `json:"fullName"`
		Username          string      `json:"username"`
		Role              domain.Role `json:"role"`
		TemporaryPassword string      `json:"temporaryPassword"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	user, err := s.deps.Users.Create(c.Request.Context(), principal(c), domain.CreateUserInput{
		FullName: request.FullName, Username: request.Username,
		Role: request.Role, TemporaryPassword: request.TemporaryPassword,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusCreated, user)
}

func (s *Server) updateUser(c *gin.Context) {
	id, err := parseUUID(c.Param("userId"), "userId")
	if err != nil {
		writeError(c, err)
		return
	}
	var request struct {
		FullName *string      `json:"fullName"`
		Username *string      `json:"username"`
		Role     *domain.Role `json:"role"`
		Active   *bool        `json:"active"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	user, err := s.deps.Users.Update(c.Request.Context(), principal(c), id, domain.UpdateUserInput{
		FullName: request.FullName, Username: request.Username, Role: request.Role, IsActive: request.Active,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, user)
}

func (s *Server) resetPassword(c *gin.Context) {
	id, err := parseUUID(c.Param("userId"), "userId")
	if err != nil {
		writeError(c, err)
		return
	}
	var request struct {
		TemporaryPassword string `json:"temporaryPassword"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	user, err := s.deps.Users.ResetPassword(c.Request.Context(), principal(c), id, request.TemporaryPassword)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, user)
}

func (s *Server) deleteUser(c *gin.Context) {
	id, err := parseUUID(c.Param("userId"), "userId")
	if err != nil {
		writeError(c, err)
		return
	}
	if err := s.deps.Users.Delete(c.Request.Context(), principal(c), id, "Dihapus oleh superadmin"); err != nil {
		writeError(c, err)
		return
	}
	user, err := s.deps.Repo.GetUser(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, user)
}

func (s *Server) listPackages(c *gin.Context) {
	items, err := s.deps.Packages.List(c.Request.Context(), principal(c), c.Query("includeDeleted") == "true")
	if err != nil {
		writeError(c, err)
		return
	}
	views := make([]gin.H, 0, len(items))
	for _, item := range items {
		views = append(views, packageView(item))
	}
	writePage(c, views, "")
}

func (s *Server) getPackage(c *gin.Context) {
	id, err := parseUUID(c.Param("packageId"), "packageId")
	if err != nil {
		writeError(c, err)
		return
	}
	item, err := s.deps.Packages.Get(c.Request.Context(), principal(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, packageView(item))
}

func (s *Server) createPackage(c *gin.Context) {
	var request struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		UnitPrice   int64  `json:"unitPrice"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	item, err := s.deps.Packages.Create(c.Request.Context(), principal(c), domain.CreatePackageInput{
		Code: codeFromName(request.Name), Name: request.Name,
		Description: request.Description, UnitPrice: request.UnitPrice,
		ChangeReason: "Paket dibuat",
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusCreated, packageView(item))
}

func (s *Server) updatePackage(c *gin.Context) {
	id, err := parseUUID(c.Param("packageId"), "packageId")
	if err != nil {
		writeError(c, err)
		return
	}
	var request struct {
		BaseRevision *int    `json:"baseRevision"`
		Name         *string `json:"name"`
		Description  *string `json:"description"`
		UnitPrice    *int64  `json:"unitPrice"`
		Active       *bool   `json:"active"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	before, err := s.deps.Packages.Get(c.Request.Context(), principal(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	if request.BaseRevision != nil && *request.BaseRevision != before.CurrentRevision {
		writeError(c, &domain.Error{
			Code: domain.CodeRevisionConflict, Message: "Paket telah berubah di server",
			Details: map[string]any{"baseRevision": *request.BaseRevision, "currentRevision": before.CurrentRevision},
		})
		return
	}
	if request.Active != nil && !*request.Active {
		if err := s.deps.Packages.Delete(c.Request.Context(), principal(c), id, "Paket dinonaktifkan"); err != nil {
			writeError(c, err)
			return
		}
		deleted, getErr := s.deps.Repo.GetPackage(
			c.Request.Context(),
			principal(c).EffectiveDataSpaceID(),
			id,
		)
		if getErr != nil {
			writeError(c, getErr)
			return
		}
		writeData(c, http.StatusOK, packageView(deleted))
		return
	}
	name, description, price := before.Name, before.Description, before.UnitPrice
	if request.Name != nil {
		name = *request.Name
	}
	if request.Description != nil {
		description = *request.Description
	}
	if request.UnitPrice != nil {
		price = *request.UnitPrice
	}
	item, err := s.deps.Packages.Update(c.Request.Context(), principal(c), id, domain.UpdatePackageInput{
		Name: name, Description: description, UnitPrice: price, ChangeReason: "Perubahan paket",
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, packageView(item))
}

func (s *Server) deletePackage(c *gin.Context) {
	id, err := parseUUID(c.Param("packageId"), "packageId")
	if err != nil {
		writeError(c, err)
		return
	}
	if err := s.deps.Packages.Delete(c.Request.Context(), principal(c), id, "Paket dihapus"); err != nil {
		writeError(c, err)
		return
	}
	item, err := s.deps.Repo.GetPackage(c.Request.Context(), principal(c).EffectiveDataSpaceID(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, packageView(item))
}

func packageView(item domain.Package) gin.H {
	revisionID := uuid.NewSHA1(item.ID, []byte(strconv.Itoa(item.CurrentRevision)))
	return gin.H{
		"id": item.ID, "active": item.DeletedAt == nil,
		"currentRevision": gin.H{
			"id": revisionID, "packageId": item.ID, "revision": item.CurrentRevision,
			"name": item.Name, "description": item.Description, "unitPrice": item.UnitPrice,
			"createdAt": item.UpdatedAt,
			"createdBy": gin.H{
				"id": uuid.Nil, "fullName": "Sistem", "username": "sistem", "role": domain.RoleSuperadmin,
			},
		},
		"createdAt": item.CreatedAt, "updatedAt": item.UpdatedAt, "deletedAt": item.DeletedAt,
	}
}

var nonCode = regexp.MustCompile(`[^A-Z0-9]+`)

func codeFromName(name string) string {
	code := strings.Trim(nonCode.ReplaceAllString(strings.ToUpper(strings.TrimSpace(name)), "_"), "_")
	if len(code) > 32 {
		code = code[:32]
	}
	if len(code) < 2 {
		code = fmt.Sprintf("PKG_%s", strings.ToUpper(uuid.NewString()[:8]))
	}
	return code
}
