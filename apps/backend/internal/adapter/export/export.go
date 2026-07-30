// Package export builds deterministic, dependency-free XLSX and PDF exports.
package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
)

type Generator struct{}

func (Generator) XLSX(rows []domain.ExportRow, from, to *time.Time) ([]byte, error) {
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`,
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets><sheet name="Transaksi" sheetId="1" r:id="rId1"/></sheets>
</workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`,
		"xl/styles.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <numFmts count="1"><numFmt numFmtId="164" formatCode="#,##0"/></numFmts>
  <fonts count="2"><font><sz val="11"/><name val="Arial"/></font><font><b/><color rgb="FFFFFFFF"/><sz val="11"/><name val="Arial"/></font></fonts>
  <fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF003D9B"/></patternFill></fill></fills>
  <borders count="1"><border/></borders>
  <cellStyleXfs count="1"><xf/></cellStyleXfs>
  <cellXfs count="3"><xf fontId="0" fillId="0" borderId="0"/><xf fontId="1" fillId="2" borderId="0" applyFill="1" applyFont="1"/><xf fontId="0" fillId="0" borderId="0" numFmtId="164" applyNumberFormat="1"/></cellXfs>
</styleSheet>`,
		"xl/worksheets/sheet1.xml": buildSheet(rows),
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		writer, err := archive.Create(name)
		if err != nil {
			return nil, fmt.Errorf("create xlsx entry %s: %w", name, err)
		}
		if _, err := writer.Write([]byte(files[name])); err != nil {
			return nil, fmt.Errorf("write xlsx entry %s: %w", name, err)
		}
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("close xlsx: %w", err)
	}
	return output.Bytes(), nil
}

func buildSheet(rows []domain.ExportRow) string {
	headers := []string{
		"ID Transaksi", "Waktu (Asia/Jakarta)", "Revisi", "Kode Paket", "Nama Paket",
		"Revisi Paket", "Harga Satuan", "Jumlah", "Total Baris", "Total Transaksi",
		"Pembuat", "Username", "Metode Pembayaran", "QRIS Payload Hash",
		"Status Pembayaran", "Status Cetak",
	}
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	body.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews><cols>`)
	for index := range headers {
		width := 15
		if index == 0 || index == 1 {
			width = 30
		}
		body.WriteString(fmt.Sprintf(`<col min="%d" max="%d" width="%d" customWidth="1"/>`, index+1, index+1, width))
	}
	body.WriteString(`</cols><sheetData><row r="1">`)
	for index, header := range headers {
		body.WriteString(inlineCell(cellName(index+1, 1), header, 1))
	}
	body.WriteString(`</row>`)
	location, _ := time.LoadLocation("Asia/Jakarta")
	for rowIndex, row := range rows {
		number := rowIndex + 2
		body.WriteString(fmt.Sprintf(`<row r="%d">`, number))
		values := []any{
			"TRX-" + row.TransactionID,
			row.OccurredAt.In(location).Format("02-01-2006 15:04:05"),
			row.Revision,
			row.PackageCode,
			row.PackageName,
			row.PackageRevision,
			row.UnitPrice,
			row.Quantity,
			row.LineTotal,
			row.TransactionTotal,
			row.CreatorName,
			row.CreatorUsername,
			string(row.PaymentMethod),
			stringPointerValue(row.QrisPayloadHash),
			string(row.PaymentStatus),
			row.PrintState,
		}
		for column, value := range values {
			cell := cellName(column+1, number)
			switch typed := value.(type) {
			case int:
				body.WriteString(numberCell(cell, int64(typed), moneyColumn(column)))
			case int64:
				body.WriteString(numberCell(cell, typed, moneyColumn(column)))
			default:
				body.WriteString(inlineCell(cell, fmt.Sprint(value), 0))
			}
		}
		body.WriteString(`</row>`)
	}
	lastRow := len(rows) + 1
	body.WriteString(fmt.Sprintf(`</sheetData><autoFilter ref="A1:P%d"/></worksheet>`, lastRow))
	return body.String()
}

func moneyColumn(zeroBased int) bool {
	return zeroBased == 6 || zeroBased == 8 || zeroBased == 9
}

func inlineCell(reference, value string, style int) string {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(value))
	return fmt.Sprintf(`<c r="%s" t="inlineStr" s="%d"><is><t>%s</t></is></c>`, reference, style, escaped.String())
}

func numberCell(reference string, value int64, money bool) string {
	style := 0
	if money {
		style = 2
	}
	return fmt.Sprintf(`<c r="%s" s="%d"><v>%d</v></c>`, reference, style, value)
}

func cellName(column, row int) string {
	var letters []byte
	for column > 0 {
		column--
		letters = append([]byte{byte('A' + column%26)}, letters...)
		column /= 26
	}
	return string(letters) + strconv.Itoa(row)
}

func (Generator) PDF(rows []domain.ExportRow, from, to *time.Time) ([]byte, error) {
	location, _ := time.LoadLocation("Asia/Jakarta")
	lines := []string{"SEWA MOTOR POS - LAPORAN TRANSAKSI"}
	if from != nil && to != nil {
		lines = append(lines, fmt.Sprintf("Periode: %s s.d. %s",
			from.In(location).Format("02-01-2006"),
			to.Add(-time.Nanosecond).In(location).Format("02-01-2006"),
		))
	}
	seen := make(map[string]struct{})
	var total int64
	for _, row := range rows {
		firstLineForTransaction := false
		if _, ok := seen[row.TransactionID]; !ok {
			seen[row.TransactionID] = struct{}{}
			firstLineForTransaction = true
			if row.PaymentStatus == domain.PaymentStatusSuccess {
				total += row.TransactionTotal
			}
		}
		lines = append(lines, fmt.Sprintf(
			"TRX-%s | %s | %s | %d x Rp%s | Rp%s | %s | %s",
			row.TransactionID,
			row.OccurredAt.In(location).Format("02-01-2006 15:04"),
			row.PackageName,
			row.Quantity,
			formatInteger(row.UnitPrice),
			formatInteger(row.LineTotal),
			row.PaymentMethod,
			row.PaymentStatus,
		))
		if firstLineForTransaction && row.QrisPayloadHash != nil {
			lines = append(lines, "QRIS payload hash: "+*row.QrisPayloadHash)
		}
	}
	lines = append(lines,
		fmt.Sprintf("Jumlah transaksi: %d", len(seen)),
		fmt.Sprintf("Pendapatan bruto: Rp%s", formatInteger(total)),
	)
	return minimalPDF(lines), nil
}

func minimalPDF(lines []string) []byte {
	const linesPerPage = 48
	pageCount := (len(lines) + linesPerPage - 1) / linesPerPage
	if pageCount == 0 {
		pageCount = 1
	}
	objects := make([]string, 3+pageCount*2)
	objects[0] = `<< /Type /Catalog /Pages 2 0 R >>`
	pageRefs := make([]string, pageCount)
	for page := 0; page < pageCount; page++ {
		pageObject := 4 + page*2
		contentObject := pageObject + 1
		pageRefs[page] = fmt.Sprintf("%d 0 R", pageObject)
		start := page * linesPerPage
		end := start + linesPerPage
		if end > len(lines) {
			end = len(lines)
		}
		var content strings.Builder
		content.WriteString("BT /F1 9 Tf 36 806 Td 12 TL ")
		for _, line := range lines[start:end] {
			if len(line) > 116 {
				line = line[:116]
			}
			content.WriteString("(" + pdfEscape(line) + ") Tj T* ")
		}
		content.WriteString("ET")
		stream := content.String()
		objects[pageObject-1] = fmt.Sprintf(
			`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>`,
			contentObject,
		)
		objects[contentObject-1] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream)
	}
	objects[1] = fmt.Sprintf(`<< /Type /Pages /Kids [%s] /Count %d >>`, strings.Join(pageRefs, " "), pageCount)
	objects[2] = `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>`

	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for index := 1; index <= len(objects); index++ {
		fmt.Fprintf(&output, "%010d 00000 n \n", offsets[index])
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return output.Bytes()
}

func pdfEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `(`, `\(`)
	return strings.ReplaceAll(value, `)`, `\)`)
}

func formatInteger(value int64) string {
	raw := strconv.FormatInt(value, 10)
	for index := len(raw) - 3; index > 0; index -= 3 {
		raw = raw[:index] + "." + raw[index:]
	}
	return raw
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
