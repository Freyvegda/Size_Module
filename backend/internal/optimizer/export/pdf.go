package export

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/size-module/backend/internal/optimizer/core"
)

// PDF writes a printable one-page-per-sheet drawing with the pieces, their
// codes and the reusable leftovers. It is a minimal, dependency-free PDF 1.4
// writer: A4 landscape pages, Helvetica labels, vector rectangles.
func PDF(w io.Writer, sol core.Solution, opts Options) error {
	sheets := sheetsOf(sol, opts)
	if len(sheets) == 0 {
		return fmt.Errorf("no sheets to export")
	}

	const (
		pageW  = 842.0 // A4 landscape, points
		pageH  = 595.0
		margin = 36.0
	)

	nPages := len(sheets)
	totalObjects := 3 + 2*nPages
	offsets := make([]int, totalObjects+1)

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	startObject := func(n int) {
		offsets[n] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", n)
	}
	endObject := func() { buf.WriteString("endobj\n") }

	// 1: catalog
	startObject(1)
	buf.WriteString("<< /Type /Catalog /Pages 2 0 R >>\n")
	endObject()

	// 2: page tree
	var kids strings.Builder
	for i := 0; i < nPages; i++ {
		fmt.Fprintf(&kids, "%d 0 R ", 4+2*i)
	}
	startObject(2)
	fmt.Fprintf(&buf, "<< /Type /Pages /Kids [%s] /Count %d >>\n", strings.TrimSpace(kids.String()), nPages)
	endObject()

	// 3: font
	startObject(3)
	buf.WriteString("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>\n")
	endObject()

	for i, sheet := range sheets {
		pageObj := 4 + 2*i
		contentObj := pageObj + 1
		content := pdfSheetContent(sheet, pageW, pageH, margin, sol)

		startObject(pageObj)
		fmt.Fprintf(&buf, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] ", pageW, pageH)
		buf.WriteString("/Resources << /Font << /F1 3 0 R >> >> ")
		fmt.Fprintf(&buf, "/Contents %d 0 R >>\n", contentObj)
		endObject()

		startObject(contentObj)
		fmt.Fprintf(&buf, "<< /Length %d >>\nstream\n", len(content))
		buf.WriteString(content)
		buf.WriteString("\nendstream\n")
		endObject()
	}

	// Cross-reference table and trailer.
	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", totalObjects+1)
	buf.WriteString("0000000000 65535 f \n")
	for n := 1; n <= totalObjects; n++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[n])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", totalObjects+1, xrefOffset)

	_, err := w.Write(buf.Bytes())
	return err
}

// pdfSheetContent builds the content stream of one page: the sheet rectangle,
// the pieces, the offcuts and the labels.
func pdfSheetContent(sheet core.SheetPlan, pageW, pageH, margin float64, sol core.Solution) string {
	sheetW := mmf(sheet.Width)
	sheetH := mmf(sheet.Height)
	if sheetW <= 0 || sheetH <= 0 {
		sheetW, sheetH = 1, 1
	}

	const titleSpace = 26.0
	scale := (pageW - 2*margin) / sheetW
	if fit := (pageH - 2*margin - titleSpace) / sheetH; fit < scale {
		scale = fit
	}
	originX := margin
	originY := margin

	// PDF y grows up; our layout y grows down.
	flipY := func(yMM, hMM float64) float64 { return originY + (sheetH-yMM-hMM)*scale }
	px := func(v core.Dim) float64 { return mmf(v) * scale }

	var b strings.Builder
	fmt.Fprintf(&b, "0.95 0.95 0.95 rg\n%.2f %.2f %.2f %.2f re f\n",
		originX, originY, sheetW*scale, sheetH*scale)
	fmt.Fprintf(&b, "0.2 0.2 0.2 RG 0.8 w\n%.2f %.2f %.2f %.2f re S\n",
		originX, originY, sheetW*scale, sheetH*scale)

	fmt.Fprintf(&b, "BT /F1 11 Tf 0.1 0.1 0.1 rg %.2f %.2f Td (%s) Tj ET\n",
		originX, pageH-margin+2, pdfText(fmt.Sprintf("Sheet %d  ·  %s  ·  %s × %s mm  ·  %s",
			sheet.Index+1, sheet.Label, mm(sheet.Width), mm(sheet.Height), sol.Solver)))

	for _, pl := range sheet.Placements {
		x := originX + mmf(pl.X)*scale
		y := flipY(mmf(pl.Y), mmf(pl.H))
		w := px(pl.W)
		h := px(pl.H)
		fmt.Fprintf(&b, "0.75 0.82 0.92 rg %.2f %.2f %.2f %.2f re f\n", x, y, w, h)
		fmt.Fprintf(&b, "0.15 0.25 0.45 RG 0.6 w %.2f %.2f %.2f %.2f re S\n", x, y, w, h)
		if w > 34 && h > 10 {
			font := 7.0
			if h > 26 {
				font = 8.5
			}
			fmt.Fprintf(&b, "BT /F1 %.1f Tf 0.1 0.1 0.1 rg %.2f %.2f Td (%s) Tj ET\n",
				font, x+2, y+font*0.4+1, pdfText(truncate(pl.PartCode, int(w/3.5))))
		}
	}
	for _, off := range sheet.Offcuts {
		x := originX + mmf(off.X)*scale
		y := flipY(mmf(off.Y), mmf(off.H))
		w := px(off.W)
		h := px(off.H)
		fmt.Fprintf(&b, "0.82 0.95 0.88 rg 0.1 0.6 0.4 RG 0.6 w %.2f %.2f %.2f %.2f re B\n", x, y, w, h)
	}
	if len(sheet.CutSteps) > 0 {
		fmt.Fprintf(&b, "BT /F1 7 Tf 0.35 0.35 0.35 rg %.2f %.2f Td (%s) Tj ET\n",
			originX, originY-11, pdfText(fmt.Sprintf("%d cut step(s) — see the CSV cut list", len(sheet.CutSteps))))
	}
	return b.String()
}

// pdfText escapes a string for a PDF literal string and keeps it ASCII.
func pdfText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\' || r == '(' || r == ')':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 32 || r > 126:
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
