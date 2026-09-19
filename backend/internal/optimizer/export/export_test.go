package export

import (
	"bytes"
	"encoding/csv"
	"strconv"
	"strings"
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
)

func testSolution() core.Solution {
	return core.Solution{
		Solver:        "shelf-2d",
		SolverVersion: "0.1.0",
		Sheets: []core.SheetPlan{
			{
				Index: 0, StockID: "sheet", StockCode: "SHEET-1000x1000", Label: "SHEET-1000x1000",
				Width: core.FromMM(1000), Height: core.FromMM(1000),
				Placements: []core.Placement{
					{PartID: "a", PartCode: "PANE-A", X: core.FromMM(10), Y: core.FromMM(10), W: core.FromMM(400), H: core.FromMM(300), Locked: true},
					{PartID: "b", PartCode: "PANE-B", X: core.FromMM(414), Y: core.FromMM(10), W: core.FromMM(300), H: core.FromMM(400), Rotated: true},
				},
				Offcuts:  []core.Rect{{X: core.FromMM(10), Y: core.FromMM(414), W: core.FromMM(980), H: core.FromMM(576)}},
				CutSteps: []string{"vertical (rip) cut at 400 mm from the region edge, kerf 4 mm"},
			},
			{
				Index: 1, StockID: "bar", StockCode: "BAR-6000", Label: "OFF-BAR",
				Width: core.FromMM(6000), Height: core.FromMM(60),
				Placements: []core.Placement{
					{PartID: "c", PartCode: "RAIL", X: core.FromMM(10), Y: 0, W: core.FromMM(1200), H: core.FromMM(60)},
				},
			},
		},
		Unplaced: []core.UnplacedPart{{PartID: "d", PartCode: "PANE-C", Quantity: 2, Reason: "no stock or remaining capacity"}},
	}
}

func TestCSVCutList(t *testing.T) {
	var buf bytes.Buffer
	if err := CSV(&buf, testSolution(), Options{}); err != nil {
		t.Fatalf("csv: %v", err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records) < 8 {
		t.Fatalf("csv has %d rows, expected the header plus sections", len(records))
	}
	if records[0][0] != "sheet" || records[0][3] != "kind" {
		t.Fatalf("unexpected header: %v", records[0])
	}
	kinds := map[string]int{}
	locked := ""
	for _, row := range records[1:] {
		kinds[row[3]]++
		if row[5] == "PANE-A" {
			locked = row[11]
		}
	}
	if kinds["sheet"] != 2 || kinds["piece"] != 3 || kinds["cut"] != 1 || kinds["offcut"] != 1 || kinds["unplaced"] != 1 {
		t.Fatalf("unexpected section counts: %v", kinds)
	}
	if locked != "true" {
		t.Fatalf("locked placement not reported: %q", locked)
	}
}

func TestExportsCanSelectOneSheet(t *testing.T) {
	one := 1
	for _, format := range []Format{FormatCSV, FormatSVG, FormatDXF, FormatPDF} {
		var buf bytes.Buffer
		if err := Write(&buf, format, testSolution(), Options{Sheet: &one}); err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		if strings.Contains(buf.String(), "PANE-A") {
			t.Fatalf("%s leaked sheet 0 when sheet 1 was selected", format)
		}
	}
}

func TestSVGDrawing(t *testing.T) {
	var buf bytes.Buffer
	if err := SVG(&buf, testSolution(), Options{}); err != nil {
		t.Fatalf("svg: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"<svg", "PANE-A", "offcut", "Sheet 2", "</svg>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("svg is missing %q", want)
		}
	}
}

func TestDXFDrawing(t *testing.T) {
	var buf bytes.Buffer
	if err := DXF(&buf, testSolution(), Options{}); err != nil {
		t.Fatalf("dxf: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"SECTION", "ENTITIES", "AC1009", "LINE", "PANE-A", "OFFCUT", "EOF"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dxf is missing %q", want)
		}
	}
}

func TestPDFStructure(t *testing.T) {
	var buf bytes.Buffer
	if err := PDF(&buf, testSolution(), Options{}); err != nil {
		t.Fatalf("pdf: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "%PDF-1.4") {
		t.Fatal("pdf header is missing")
	}
	if !strings.Contains(out, "%%EOF") || !strings.Contains(out, "/BaseFont /Helvetica") {
		t.Fatal("pdf trailer or font is missing")
	}
	// The cross-reference offset must point at the xref table.
	idx := strings.LastIndex(out, "startxref")
	if idx < 0 {
		t.Fatal("no startxref")
	}
	offsetText := strings.TrimSpace(out[idx+len("startxref"):])
	offsetText = strings.TrimSuffix(offsetText, "%%EOF")
	offset, err := strconv.Atoi(strings.TrimSpace(offsetText))
	if err != nil {
		t.Fatalf("bad startxref: %v", err)
	}
	if offset <= 0 || offset >= len(out) || !strings.HasPrefix(out[offset:], "xref") {
		t.Fatalf("startxref points to %q", out[offset:min(offset+8, len(out))])
	}
}

func TestUnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, "docx", testSolution(), Options{}); err == nil {
		t.Fatal("expected an error for an unsupported format")
	}
	if Supported("docx") || !Supported("pdf") {
		t.Fatal("Supported() is wrong")
	}
}
