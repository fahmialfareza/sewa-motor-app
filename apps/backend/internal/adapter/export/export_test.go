package export

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
)

func sampleRows() []domain.ExportRow {
	return []domain.ExportRow{{
		TransactionID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		OccurredAt:    time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC),
		Revision:      1, PackageCode: "STANDARD", PackageName: "Paket Standar",
		PackageRevision: 1, UnitPrice: 70_000, Quantity: 2, LineTotal: 140_000,
		TransactionTotal: 140_000, CreatorName: "Admin", CreatorUsername: "admin",
		PrintState: "success",
	}}
}

func TestXLSXIsReadableOpenXMLArchive(t *testing.T) {
	body, err := (Generator{}).XLSX(sampleRows(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	foundSheet := false
	for _, file := range archive.File {
		if file.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, _ := io.ReadAll(reader)
		reader.Close()
		foundSheet = bytes.Contains(content, []byte("Paket Standar")) &&
			bytes.Contains(content, []byte("TRX-01ARZ3NDEKTSV4RRFFQ69G5FAV"))
	}
	if !foundSheet {
		t.Fatal("worksheet or expected data missing")
	}
}

func TestPDFHasValidEnvelopeAndTotals(t *testing.T) {
	body, err := (Generator{}).PDF(sampleRows(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(body, []byte("%PDF-1.4")) || !bytes.HasSuffix(body, []byte("%%EOF\n")) {
		t.Fatal("invalid PDF envelope")
	}
	if !bytes.Contains(body, []byte("Pendapatan bruto: Rp140.000")) {
		t.Fatal("gross revenue missing")
	}
}
