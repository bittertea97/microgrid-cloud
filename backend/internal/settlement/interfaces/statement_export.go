package interfaces

import (
	"bytes"
	"fmt"
	"time"

	"github.com/jung-kurt/gofpdf"
	"github.com/xuri/excelize/v2"

	settlement "microgrid-cloud/internal/settlement/domain"
)

// BuildStatementPDF renders a minimal PDF for a statement.
func BuildStatementPDF(stmt *settlement.StatementAggregate, items []settlement.StatementItem) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetFont("Arial", "", 12)
	pdf.AddPage()

	pdf.Cell(0, 8, "Settlement Statement")
	pdf.Ln(10)
	pdf.SetFont("Arial", "", 10)
	pdf.Cell(0, 6, fmt.Sprintf("Station: %s", stmt.StationID))
	pdf.Ln(5)
	pdf.Cell(0, 6, fmt.Sprintf("Month: %s", stmt.StatementMonth.Format("2006-01")))
	pdf.Ln(5)
	pdf.Cell(0, 6, fmt.Sprintf("Category: %s", stmt.Category))
	pdf.Ln(5)
	pdf.Cell(0, 6, fmt.Sprintf("Version: %d", stmt.Version))
	pdf.Ln(5)
	pdf.Cell(0, 6, fmt.Sprintf("Status: %s", stmt.Status))
	pdf.Ln(5)
	pdf.Cell(0, 6, fmt.Sprintf("Generated: %s", stmt.CreatedAt.Format(time.RFC3339)))
	pdf.Ln(5)
	if !stmt.FrozenAt.IsZero() {
		pdf.Cell(0, 6, fmt.Sprintf("Frozen: %s", stmt.FrozenAt.Format(time.RFC3339)))
		pdf.Ln(5)
	}

	pdf.Ln(4)
	pdf.Cell(0, 6, fmt.Sprintf("Total Energy (kWh): %.3f", stmt.TotalEnergyKWh))
	pdf.Ln(5)
	pdf.Cell(0, 6, fmt.Sprintf("Total Amount (%s): %.2f", stmt.Currency, stmt.TotalAmount))
	pdf.Ln(8)

	// Items table
	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(40, 6, "Day", "1", 0, "C", false, 0, "")
	pdf.CellFormat(50, 6, "Energy (kWh)", "1", 0, "C", false, 0, "")
	pdf.CellFormat(50, 6, "Amount", "1", 0, "C", false, 0, "")
	pdf.Ln(-1)
	pdf.SetFont("Arial", "", 10)
	for _, item := range items {
		pdf.CellFormat(40, 6, item.DayStart.Format("2006-01-02"), "1", 0, "C", false, 0, "")
		pdf.CellFormat(50, 6, fmt.Sprintf("%.3f", item.EnergyKWh), "1", 0, "R", false, 0, "")
		pdf.CellFormat(50, 6, fmt.Sprintf("%.2f", item.Amount), "1", 0, "R", false, 0, "")
		pdf.Ln(-1)
	}

	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BuildStatementXLSX renders a minimal XLSX for a statement.
func BuildStatementXLSX(stmt *settlement.StatementAggregate, items []settlement.StatementItem) ([]byte, error) {
	f := excelize.NewFile()
	summarySheet := "summary"
	itemsSheet := "items"
	f.SetSheetName("Sheet1", summarySheet)
	f.NewSheet(itemsSheet)

	_ = f.SetCellValue(summarySheet, "A1", "Settlement Statement")
	_ = f.SetCellValue(summarySheet, "A3", "Station")
	_ = f.SetCellValue(summarySheet, "B3", stmt.StationID)
	_ = f.SetCellValue(summarySheet, "A4", "Month")
	_ = f.SetCellValue(summarySheet, "B4", stmt.StatementMonth.Format("2006-01"))
	_ = f.SetCellValue(summarySheet, "A5", "Category")
	_ = f.SetCellValue(summarySheet, "B5", stmt.Category)
	_ = f.SetCellValue(summarySheet, "A6", "Version")
	_ = f.SetCellValue(summarySheet, "B6", stmt.Version)
	_ = f.SetCellValue(summarySheet, "A7", "Status")
	_ = f.SetCellValue(summarySheet, "B7", stmt.Status)
	_ = f.SetCellValue(summarySheet, "A8", "Total Energy (kWh)")
	_ = f.SetCellValue(summarySheet, "B8", stmt.TotalEnergyKWh)
	_ = f.SetCellValue(summarySheet, "A9", "Total Amount")
	_ = f.SetCellValue(summarySheet, "B9", stmt.TotalAmount)
	_ = f.SetCellValue(summarySheet, "A10", "Currency")
	_ = f.SetCellValue(summarySheet, "B10", stmt.Currency)

	_ = f.SetCellValue(itemsSheet, "A1", "Day")
	_ = f.SetCellValue(itemsSheet, "B1", "Energy (kWh)")
	_ = f.SetCellValue(itemsSheet, "C1", "Amount")
	for i, item := range items {
		row := i + 2
		_ = f.SetCellValue(itemsSheet, fmt.Sprintf("A%d", row), item.DayStart.Format("2006-01-02"))
		_ = f.SetCellValue(itemsSheet, fmt.Sprintf("B%d", row), item.EnergyKWh)
		_ = f.SetCellValue(itemsSheet, fmt.Sprintf("C%d", row), item.Amount)
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
