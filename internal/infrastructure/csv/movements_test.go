package csv

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

func TestParseRowsRoundTripsBuildTemplate(t *testing.T) {
	template := BuildTemplate("usd")

	rows, err := ParseRows(strings.NewReader(template))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Date != "2025-11-03" || rows[0].Currency != "usd" || rows[0].Amount != "-4590" {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
}

func TestParseRowsRejectsWrongHeader(t *testing.T) {
	_, err := ParseRows(strings.NewReader("a,b,c\n1,2,3\n"))
	if err == nil {
		t.Fatal("want error for wrong header, got nil")
	}
}

func TestParseRowsRejectsEmptyFile(t *testing.T) {
	_, err := ParseRows(strings.NewReader(""))
	if err == nil {
		t.Fatal("want error for empty file, got nil")
	}
}

func TestWriteMovementsThenParseRowsRoundTrips(t *testing.T) {
	accountID := "acc-1"
	movements := []*dto.MovementDTO{
		{
			Amount: -4590, Currency: "usd", Description: "Groceries",
			Category: "food", PaymentMethod: "debit_card",
			AccountID: &accountID,
			Timestamp: time.Date(2025, 11, 3, 12, 0, 0, 0, time.UTC),
		},
		{
			Amount: 250000, Currency: "usd", Description: "Salary",
			Category: "income", PaymentMethod: "bank_transfer",
			Timestamp: time.Date(2025, 11, 5, 12, 0, 0, 0, time.UTC),
		},
	}
	accountNames := map[string]string{"acc-1": "Checking"}

	var buf bytes.Buffer
	if err := WriteMovements(&buf, movements, accountNames); err != nil {
		t.Fatal(err)
	}

	rows, err := ParseRows(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Account != "Checking" {
		t.Fatalf("row 0 account = %q, want %q", rows[0].Account, "Checking")
	}
	if rows[1].Account != "" {
		t.Fatalf("row 1 account = %q, want blank (no AccountID)", rows[1].Account)
	}
	if rows[0].Date != "2025-11-03" || rows[0].Amount != "-4590" || rows[0].Currency != "usd" {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
}
