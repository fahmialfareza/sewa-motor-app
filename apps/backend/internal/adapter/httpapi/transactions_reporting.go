package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *Server) createTransaction(c *gin.Context) {
	var request struct {
		ID         string             `json:"id"`
		OccurredAt time.Time          `json:"occurredAt"`
		Items      []domain.ItemInput `json:"items"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	item, err := s.deps.Transactions.Create(c.Request.Context(), principal(c), domain.CreateTransactionInput{
		ID: request.ID, OccurredAt: request.OccurredAt, Items: request.Items,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	view, err := s.transactionView(c, item)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusCreated, view)
}

func (s *Server) getTransaction(c *gin.Context) {
	item, err := s.deps.Transactions.Get(c.Request.Context(), principal(c), c.Param("transactionId"))
	if err != nil {
		writeError(c, err)
		return
	}
	view, err := s.transactionView(c, item)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, view)
}

func (s *Server) listTransactions(c *gin.Context) {
	filter, err := transactionFilterFromQuery(c)
	if err != nil {
		writeError(c, err)
		return
	}
	if cursor := c.Query("cursor"); cursor != "" {
		decoded, cursorErr := domain.DecodeCursor(cursor)
		if cursorErr != nil {
			writeError(c, domain.Validation("Cursor transaksi tidak valid", nil))
			return
		}
		filter.CursorOccurred = &decoded.OccurredAt
		filter.CursorID = decoded.ID
	}
	page, err := s.deps.Transactions.List(c.Request.Context(), principal(c), filter)
	if err != nil {
		writeError(c, err)
		return
	}
	views := make([]gin.H, 0, len(page.Transactions))
	for _, item := range page.Transactions {
		view, viewErr := s.transactionView(c, item)
		if viewErr != nil {
			writeError(c, viewErr)
			return
		}
		views = append(views, view)
	}
	writePage(c, views, page.NextCursor)
}

func transactionFilterFromQuery(c *gin.Context) (domain.TransactionFilter, error) {
	filter := domain.TransactionFilter{
		Search: c.Query("search"), IncludeDeleted: c.Query("includeDeleted") == "true", Limit: 25,
	}
	if value := c.Query("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 100 {
			return filter, domain.Validation("Limit harus antara 1 dan 100", map[string]any{"field": "limit"})
		}
		filter.Limit = limit
	}
	for field, target := range map[string]**time.Time{"from": &filter.From, "to": &filter.To} {
		if value := c.Query(field); value != "" {
			parsed, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return filter, domain.Validation("Waktu filter tidak valid", map[string]any{"field": field})
			}
			*target = &parsed
		}
	}
	for field, target := range map[string]**uuid.UUID{
		"packageId": &filter.PackageID, "creatorId": &filter.CreatorID, "terminalId": &filter.TerminalID,
	} {
		if value := c.Query(field); value != "" {
			id, err := parseUUID(value, field)
			if err != nil {
				return filter, err
			}
			*target = &id
		}
	}
	return filter, nil
}

func (s *Server) correctTransaction(c *gin.Context) {
	var request struct {
		BaseRevision int                `json:"baseRevision"`
		Reason       string             `json:"reason"`
		OccurredAt   time.Time          `json:"occurredAt"`
		Items        []domain.ItemInput `json:"items"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	item, err := s.deps.Transactions.Correct(c.Request.Context(), principal(c), domain.CorrectTransactionInput{
		ID: c.Param("transactionId"), BaseRevision: request.BaseRevision, Reason: request.Reason,
		OccurredAt: request.OccurredAt, Items: request.Items,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	view, err := s.transactionView(c, item)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, view)
}

func (s *Server) deleteTransaction(c *gin.Context) {
	var request struct {
		BaseRevision int    `json:"baseRevision"`
		Reason       string `json:"reason"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	before, err := s.deps.Transactions.Get(c.Request.Context(), principal(c), c.Param("transactionId"))
	if err != nil {
		writeError(c, err)
		return
	}
	if before.Revision != request.BaseRevision {
		writeError(c, &domain.Error{
			Code: domain.CodeRevisionConflict, Message: "Transaksi telah berubah di server",
			Details: map[string]any{"baseRevision": request.BaseRevision, "currentRevision": before.Revision},
		})
		return
	}
	if err := s.deps.Transactions.Delete(c.Request.Context(), principal(c), before.ID, request.Reason); err != nil {
		writeError(c, err)
		return
	}
	deleted, err := s.deps.Repo.GetTransaction(c.Request.Context(), before.ID, true)
	if err != nil {
		writeError(c, err)
		return
	}
	view, err := s.transactionView(c, deleted)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, view)
}

func (s *Server) listRevisions(c *gin.Context) {
	revisions, err := s.deps.Transactions.Revisions(c.Request.Context(), principal(c), c.Param("transactionId"))
	if err != nil {
		writeError(c, err)
		return
	}
	data := make([]gin.H, 0, len(revisions))
	for _, revision := range revisions {
		origin, originErr := s.deps.Repo.GetUser(c.Request.Context(), revision.OriginActorID)
		if originErr != nil {
			writeError(c, originErr)
			return
		}
		submitter, submitErr := s.deps.Repo.GetUser(c.Request.Context(), revision.SubmittedBy)
		if submitErr != nil {
			writeError(c, submitErr)
			return
		}
		var terminal any
		if revision.TerminalID != nil {
			item, terminalErr := s.deps.Repo.GetTerminal(c.Request.Context(), *revision.TerminalID)
			if terminalErr != nil {
				writeError(c, terminalErr)
				return
			}
			terminal = terminalSummary(item)
		}
		data = append(data, gin.H{
			"id":            uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:%d", revision.TransactionID, revision.Revision))),
			"transactionId": revision.TransactionID, "revision": revision.Revision,
			"reason": revision.Reason, "before": json.RawMessage(revision.BeforeSnapshot),
			"after":       json.RawMessage(revision.AfterSnapshot),
			"originActor": actorFromUser(origin), "submittedBy": actorFromUser(submitter),
			"terminal": terminal, "clientOccurredAt": revision.ClientOccurredAt,
			"serverReceivedAt": revision.ServerReceivedAt,
		})
	}
	writeData(c, http.StatusOK, data)
}

func (s *Server) recordPrintAttempt(c *gin.Context) {
	var request domain.PrintAttemptInput
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	request.TransactionID = c.Param("transactionId")
	attempt, err := s.deps.Transactions.RecordPrint(c.Request.Context(), principal(c), request)
	if err != nil {
		writeError(c, err)
		return
	}
	view, err := s.printAttemptView(c, attempt)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusCreated, view)
}

func (s *Server) listPrintAttempts(c *gin.Context) {
	attempts, err := s.deps.Transactions.PrintAttempts(c.Request.Context(), principal(c), c.Param("transactionId"))
	if err != nil {
		writeError(c, err)
		return
	}
	views := make([]gin.H, 0, len(attempts))
	for _, attempt := range attempts {
		view, viewErr := s.printAttemptView(c, attempt)
		if viewErr != nil {
			writeError(c, viewErr)
			return
		}
		views = append(views, view)
	}
	writeData(c, http.StatusOK, views)
}

func (s *Server) transactionView(c *gin.Context, item domain.Transaction) (gin.H, error) {
	var terminal any
	if item.TerminalID != nil {
		value, err := s.deps.Repo.GetTerminal(c.Request.Context(), *item.TerminalID)
		if err != nil {
			return nil, err
		}
		terminal = terminalSummary(value)
	}
	attempts, err := s.deps.Repo.ListPrintAttempts(c.Request.Context(), item.ID)
	if err != nil {
		return nil, err
	}
	var lastAttempt any
	if len(attempts) > 0 {
		lastAttempt = attempts[len(attempts)-1].ServerReceivedAt
	}
	lines := make([]gin.H, 0, len(item.Items))
	for _, line := range item.Items {
		lines = append(lines, gin.H{
			"id":        uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:%d:%d", item.ID, item.Revision, line.LineNumber))),
			"packageId": line.PackageID, "packageRevision": line.PackageRevision,
			"name": line.PackageName, "description": line.PackageDescription,
			"unitPrice": line.UnitPrice, "quantity": line.Quantity, "lineTotal": line.LineTotal,
		})
	}
	var deletion any
	if item.DeletedAt != nil {
		deletion = gin.H{"deletedAt": item.DeletedAt, "deletedBy": item.DeletedBy, "reason": item.DeleteReason}
	}
	return gin.H{
		"id": item.ID, "revision": item.Revision, "occurredAt": item.OccurredAt,
		"items": lines, "subtotal": item.Subtotal, "total": item.Total,
		"originActor": item.OriginActor, "updatedBy": item.UpdatedBy, "terminal": terminal,
		"print":    gin.H{"state": item.PrintState, "attemptCount": len(attempts), "lastAttemptAt": lastAttempt},
		"deletion": deletion, "createdAt": item.ServerReceivedAt, "updatedAt": item.UpdatedAt,
	}, nil
}

func (s *Server) printAttemptView(c *gin.Context, attempt domain.PrintAttempt) (gin.H, error) {
	var terminal any
	if attempt.TerminalID != nil {
		item, err := s.deps.Repo.GetTerminal(c.Request.Context(), *attempt.TerminalID)
		if err != nil {
			return nil, err
		}
		terminal = terminalSummary(item)
	}
	actor, err := s.deps.Repo.GetUser(c.Request.Context(), attempt.ActorID)
	if err != nil {
		return nil, err
	}
	return gin.H{
		"id": attempt.ID, "transactionId": attempt.TransactionID,
		"transactionRevision": attempt.TransactionRevision, "status": attempt.Status,
		"printerKind": attempt.PrinterKind, "printerIdentifier": attempt.PrinterIdentifier,
		"isCopy": attempt.IsCopy, "errorCode": attempt.ErrorCode, "errorMessage": attempt.ErrorMessage,
		"metadata": attempt.Metadata, "occurredAt": attempt.ClientOccurredAt,
		"terminal": terminal, "actor": actorFromUser(actor), "serverReceivedAt": attempt.ServerReceivedAt,
	}, nil
}

func actorFromUser(user domain.User) gin.H {
	return gin.H{"id": user.ID, "fullName": user.FullName, "username": user.Username, "role": user.Role}
}

func terminalSummary(terminal domain.Terminal) gin.H {
	return gin.H{"id": terminal.ID, "installationId": terminal.InstallationID, "name": terminal.Name}
}

func (s *Server) dashboard(c *gin.Context) {
	period := c.Query("period")
	if period != "day" && period != "week" && period != "month" {
		writeError(c, domain.Validation("Period harus day, week, atau month", map[string]any{"field": "period"}))
		return
	}
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		writeError(c, domain.WrapInternal(err, "load Jakarta timezone"))
		return
	}
	anchor := time.Now().In(location)
	if value := c.Query("anchor"); value != "" {
		anchor, err = time.ParseInLocation("2006-01-02", value, location)
		if err != nil {
			writeError(c, domain.Validation("Anchor harus berupa tanggal YYYY-MM-DD", nil))
			return
		}
	}
	start := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, location)
	switch period {
	case "week":
		offset := (int(start.Weekday()) + 6) % 7
		start = start.AddDate(0, 0, -offset)
	case "month":
		start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, location)
	}
	end := start.AddDate(0, 0, 1)
	if period == "week" {
		end = start.AddDate(0, 0, 7)
	} else if period == "month" {
		end = start.AddDate(0, 1, 0)
	}
	data, err := s.deps.Reporting.Dashboard(c.Request.Context(), principal(c), start.UTC(), end.UTC(), period)
	if err != nil {
		writeError(c, err)
		return
	}
	recent := make([]gin.H, 0, len(data.RecentTransactions))
	for _, item := range data.RecentTransactions {
		view, viewErr := s.transactionView(c, item)
		if viewErr != nil {
			writeError(c, viewErr)
			return
		}
		recent = append(recent, view)
	}
	packageQuantities := make([]gin.H, 0, len(data.PackageQuantities))
	for _, quantity := range data.PackageQuantities {
		packageQuantities = append(packageQuantities, gin.H{
			"packageId": quantity.PackageID, "name": quantity.PackageName, "quantity": quantity.Quantity,
		})
	}
	trend := make([]gin.H, 0, len(data.Trend))
	for _, point := range data.Trend {
		bucketEnd := point.Bucket.AddDate(0, 0, 1)
		if period == "week" {
			bucketEnd = point.Bucket.AddDate(0, 0, 7)
		} else if period == "month" {
			bucketEnd = point.Bucket.AddDate(0, 1, 0)
		}
		trend = append(trend, gin.H{
			"start": point.Bucket, "end": bucketEnd,
			"grossRevenue": point.Total, "transactionCount": point.Count,
		})
	}
	writeData(c, http.StatusOK, gin.H{
		"period": period, "startsAt": data.From, "endsAt": data.To,
		"grossRevenue": data.GrossRevenue, "transactionCount": data.TransactionCount,
		"packageQuantities": packageQuantities, "trend": trend, "recent": recent,
	})
}

func (s *Server) exportTransactions(c *gin.Context) {
	var request struct {
		Format  string `json:"format"`
		Filters struct {
			Search         string     `json:"search"`
			From           *time.Time `json:"from"`
			To             *time.Time `json:"to"`
			PackageID      *uuid.UUID `json:"packageId"`
			CreatorID      *uuid.UUID `json:"creatorId"`
			TerminalID     *uuid.UUID `json:"terminalId"`
			IncludeDeleted bool       `json:"includeDeleted"`
		} `json:"filters"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	body, contentType, err := s.deps.Reporting.Export(c.Request.Context(), principal(c), request.Format, domain.TransactionFilter{
		Search: request.Filters.Search, From: request.Filters.From, To: request.Filters.To,
		PackageID: request.Filters.PackageID, CreatorID: request.Filters.CreatorID,
		TerminalID: request.Filters.TerminalID, IncludeDeleted: request.Filters.IncludeDeleted,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	filename := fmt.Sprintf("transaksi-%s.%s", time.Now().Format("20060102-150405"), request.Format)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, contentType, body)
}
