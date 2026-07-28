package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *Server) enrollTerminal(c *gin.Context) {
	var request struct {
		InstallationID uuid.UUID `json:"installationId"`
		Name           string    `json:"name"`
		PublicKey      string    `json:"publicKey"`
		Algorithm      string    `json:"algorithm"`
		DeviceModel    *string   `json:"deviceModel"`
		OSVersion      *string   `json:"osVersion"`
		AppVersion     *string   `json:"appVersion"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	if request.InstallationID == uuid.Nil || request.Algorithm != "Ed25519" {
		writeError(c, domain.Validation("Installation ID atau algoritma terminal tidak valid", nil))
		return
	}
	publicKey, err := base64.StdEncoding.DecodeString(request.PublicKey)
	if err != nil {
		publicKey, err = base64.RawStdEncoding.DecodeString(request.PublicKey)
	}
	if err != nil {
		writeError(c, domain.Validation("publicKey harus berupa Base64", map[string]any{"field": "publicKey"}))
		return
	}
	terminal, err := s.deps.Terminals.Enroll(c.Request.Context(), principal(c), domain.EnrollTerminalInput{
		InstallationID: request.InstallationID.String(), Name: request.Name, PublicKey: publicKey,
		DeviceModel: request.DeviceModel, OSVersion: request.OSVersion, AppVersion: request.AppVersion,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusCreated, terminal)
}

func (s *Server) currentTerminal(c *gin.Context) {
	terminal, err := s.deps.Terminals.Current(c.Request.Context(), principal(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, terminal)
}

func (s *Server) revokeTerminal(c *gin.Context) {
	id, err := parseUUID(c.Param("terminalId"), "terminalId")
	if err != nil {
		writeError(c, err)
		return
	}
	terminal, err := s.deps.Terminals.Revoke(c.Request.Context(), principal(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, terminal)
}

func (s *Server) syncPush(c *gin.Context) {
	var request struct {
		Mutations []domain.SyncMutation `json:"mutations"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	for index, operation := range request.Mutations {
		if _, err := uuid.Parse(operation.OperationID); err != nil {
			writeError(c, domain.Validation("operationId harus UUID", map[string]any{"index": index}))
			return
		}
		if strings.TrimSpace(operation.AggregateID) == "" {
			writeError(c, domain.Validation("aggregateId wajib diisi", map[string]any{"index": index}))
			return
		}
	}
	results, err := s.deps.Sync.Push(c.Request.Context(), principal(c), request.Mutations)
	if err != nil {
		writeError(c, err)
		return
	}
	mapped := make([]gin.H, 0, len(results))
	for index, result := range results {
		operation := request.Mutations[index]
		status := "applied"
		var data any
		var conflict any
		var apiError any
		if result.Error != nil {
			if result.Error.Code == domain.CodeRevisionConflict || result.Error.Code == domain.CodeConflict {
				status = "conflict"
				conflict = result.Error.Details
			} else {
				status = "rejected"
			}
			details := result.Error.Details
			if details == nil {
				details = map[string]any{}
			}
			apiError = gin.H{
				"code": result.Error.Code, "message": result.Error.Message,
				"details": details, "requestId": requestID(c),
			}
		} else {
			if result.Replayed {
				status = "duplicate"
			}
			if len(result.Data) > 0 {
				switch operation.Aggregate {
				case "transaction":
					var transaction domain.Transaction
					if err := json.Unmarshal(result.Data, &transaction); err != nil {
						writeError(c, domain.WrapInternal(err, "decode stored transaction result"))
						return
					}
					data, err = s.transactionView(c, transaction)
				case "print_attempt":
					var attempt domain.PrintAttempt
					if err := json.Unmarshal(result.Data, &attempt); err != nil {
						writeError(c, domain.WrapInternal(err, "decode stored print result"))
						return
					}
					data, err = s.printAttemptView(c, attempt)
				default:
					err = domain.NewError(domain.CodeInternal, "Jenis hasil sinkronisasi tidak dikenal")
				}
				if err != nil {
					writeError(c, err)
					return
				}
			}
		}
		mapped = append(mapped, gin.H{
			"operationId": result.OperationID, "aggregateId": operation.AggregateID,
			"status": status, "replayed": result.Replayed, "data": data,
			"conflict": conflict, "error": apiError,
		})
	}
	writeData(c, http.StatusOK, gin.H{"results": mapped})
}

func (s *Server) syncPull(c *gin.Context) {
	var cursor int64
	var err error
	if value := c.Query("cursor"); value != "" {
		cursor, err = strconv.ParseInt(value, 10, 64)
		if err != nil || cursor < 0 {
			writeError(c, domain.Validation("Cursor sinkronisasi tidak valid", map[string]any{"field": "cursor"}))
			return
		}
	}
	limit := 200
	if value := c.Query("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 500 {
			writeError(c, domain.Validation("Limit sinkronisasi harus antara 1 dan 500", nil))
			return
		}
	}
	changes, next, hasMore, err := s.deps.Sync.Pull(c.Request.Context(), principal(c), cursor, limit)
	if err != nil {
		writeError(c, err)
		return
	}
	mapped := make([]gin.H, 0, len(changes))
	for _, change := range changes {
		var payload any
		if !change.Tombstone {
			payload, err = s.syncChangePayload(c, change)
			if err != nil {
				writeError(c, err)
				return
			}
		}
		mapped = append(mapped, gin.H{
			"cursor": strconv.FormatInt(change.Cursor, 10), "aggregate": change.Aggregate,
			"action": change.Action, "aggregateId": change.AggregateID, "revision": change.Revision,
			"changedAt": change.CreatedAt, "tombstone": change.Tombstone, "payload": payload,
		})
	}
	writeData(c, http.StatusOK, gin.H{
		"changes": mapped, "cursor": strconv.FormatInt(next, 10), "hasMore": hasMore,
	})
}

func (s *Server) syncChangePayload(c *gin.Context, change domain.SyncChange) (any, error) {
	switch change.Aggregate {
	case "user":
		id, err := uuid.Parse(change.AggregateID)
		if err != nil {
			return nil, domain.WrapInternal(err, "parse synced user id")
		}
		return s.deps.Repo.GetUser(c.Request.Context(), id)
	case "package":
		id, err := uuid.Parse(change.AggregateID)
		if err != nil {
			return nil, domain.WrapInternal(err, "parse synced package id")
		}
		item, err := s.deps.Repo.GetPackage(c.Request.Context(), id)
		if err != nil {
			return nil, err
		}
		return packageView(item), nil
	case "transaction":
		item, err := s.deps.Repo.GetTransaction(c.Request.Context(), change.AggregateID, true)
		if err != nil {
			return nil, err
		}
		return s.transactionView(c, item)
	case "print_attempt":
		var attempt domain.PrintAttempt
		if err := json.Unmarshal(change.Payload, &attempt); err != nil {
			return nil, domain.WrapInternal(err, "decode synced print attempt")
		}
		return s.printAttemptView(c, attempt)
	case "terminal":
		id, err := uuid.Parse(change.AggregateID)
		if err != nil {
			return nil, domain.WrapInternal(err, "parse synced terminal id")
		}
		return s.deps.Repo.GetTerminal(c.Request.Context(), id)
	default:
		return nil, domain.NewError(domain.CodeInternal, "Jenis perubahan sinkronisasi tidak dikenal")
	}
}
