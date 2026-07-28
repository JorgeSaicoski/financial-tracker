// Package csv is BACK-03's CSV adapter: the fixed column model shared by
// POST /import/movements (parse) and GET /export/movements (write), kept
// here as infrastructure rather than a domain concept — CSV is an
// external file format, not something the domain layer needs to know
// about, the same way sqlite/postgresql are adapters around the
// repository contracts rather than domain types. See README.md in this
// package for the column reference.
package csv

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// Header is the fixed CSV model's column names, in this exact order.
var Header = []string{"date", "amount", "currency", "description", "category", "payment_method", "account"}

// Row is one CSV data row, before any domain validation — plain
// strings, positionally matching Header.
type Row struct {
	Date          string
	Amount        string
	Currency      string
	Description   string
	Category      string
	PaymentMethod string
	Account       string // optional; resolved case-insensitively against the user's accounts
}

// ParseRows reads and validates the fixed header, then maps every
// remaining row into a Row — it does not validate field *values* (that's
// the caller's job, row by row), only column shape.
func ParseRows(r io.Reader) ([]Row, error) {
	csvReader := csv.NewReader(r)
	csvReader.TrimLeadingSpace = true

	header, err := csvReader.Read()
	if err == io.EOF {
		return nil, errors.New("empty file: expected a header row")
	}
	if err != nil {
		return nil, fmt.Errorf("invalid CSV: %w", err)
	}
	if !headerMatches(header) {
		return nil, fmt.Errorf("invalid header: want %q, got %q", strings.Join(Header, ","), strings.Join(header, ","))
	}

	var rows []Row
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid CSV: %w", err)
		}
		if len(record) != len(Header) {
			return nil, fmt.Errorf("row %d: want %d columns, got %d", len(rows)+1, len(Header), len(record))
		}
		rows = append(rows, Row{
			Date:          record[0],
			Amount:        record[1],
			Currency:      record[2],
			Description:   record[3],
			Category:      record[4],
			PaymentMethod: record[5],
			Account:       record[6],
		})
	}
	return rows, nil
}

func headerMatches(header []string) bool {
	if len(header) != len(Header) {
		return false
	}
	for i, col := range header {
		if strings.ToLower(strings.TrimSpace(col)) != Header[i] {
			return false
		}
	}
	return true
}

// BuildTemplate returns a ready-to-copy CSV: the header plus two example
// rows, in the given currency.
func BuildTemplate(currency string) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write(Header)
	_ = w.Write([]string{"2025-11-03", "-4590", currency, "Groceries", "food", "debit_card", ""})
	_ = w.Write([]string{"2025-11-05", "250000", currency, "Salary", "income", "bank_transfer", ""})
	w.Flush()
	return b.String()
}

// WriteMovements is the revert direction of ParseRows: it renders
// movements back into the same fixed CSV model, so a file round-tripped
// through POST /import/movements and GET /export/movements reproduces
// the same rows (modulo order). accountNames maps account ID -> name for
// the optional "account" column; movements with no AccountID (or an ID
// missing from the map) get a blank account cell.
func WriteMovements(w io.Writer, movements []*dto.MovementDTO, accountNames map[string]string) error {
	csvWriter := csv.NewWriter(w)
	if err := csvWriter.Write(Header); err != nil {
		return err
	}
	for _, m := range movements {
		account := ""
		if m.AccountID != nil {
			account = accountNames[*m.AccountID]
		}
		record := []string{
			m.Timestamp.Format("2006-01-02"),
			strconv.FormatInt(m.Amount, 10),
			m.Currency,
			m.Description,
			m.Category,
			m.PaymentMethod,
			account,
		}
		if err := csvWriter.Write(record); err != nil {
			return err
		}
	}
	csvWriter.Flush()
	return csvWriter.Error()
}
