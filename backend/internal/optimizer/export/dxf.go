package export

import (
	"fmt"
	"io"
	"strings"

	"github.com/size-module/backend/internal/optimizer/core"
)

// DXF writes an AutoCAD R12 ASCII drawing with every sheet laid out side by
// side in millimetres: the sheet outline on layer SHEET, pieces on PARTS,
// reusable leftovers on OFFCUT and labels on TEXT. R12 keeps the file readable
// by essentially every CAM and CAD tool.
func DXF(w io.Writer, sol core.Solution, opts Options) error {
	sheets := sheetsOf(sol, opts)
	if len(sheets) == 0 {
		return fmt.Errorf("no sheets to export")
	}

	const gapMM = 200.0
	var b strings.Builder
	b.WriteString("0\nSECTION\n2\nENTITIES\n")

	x := 0.0
	maxX, maxY := 0.0, 0.0
	for _, sheet := range sheets {
		sheetW := mmf(sheet.Width)
		sheetH := mmf(sheet.Height)

		// DXF y grows upward; the layout is top-down, so flip inside the sheet.
		flip := func(yMM, hMM float64) float64 { return sheetH - yMM - hMM }
		rect := func(x0, y0, w0, h0 float64, layer string) {
			writeLine(&b, layer, x0, y0, x0+w0, y0)
			writeLine(&b, layer, x0+w0, y0, x0+w0, y0+h0)
			writeLine(&b, layer, x0+w0, y0+h0, x0, y0+h0)
			writeLine(&b, layer, x0, y0+h0, x0, y0)
		}

		rect(x, 0, sheetW, sheetH, "SHEET")
		writeText(&b, "TEXT", x, sheetH+30, 12, fmt.Sprintf("Sheet %d - %s", sheet.Index+1, sheet.Label))

		for _, pl := range sheet.Placements {
			px := x + mmf(pl.X)
			py := flip(mmf(pl.Y), mmf(pl.H))
			pw := mmf(pl.W)
			ph := mmf(pl.H)
			rect(px, py, pw, ph, "PARTS")
			writeText(&b, "TEXT", px+2, py+ph/2, 6, pl.PartCode)
		}
		for _, off := range sheet.Offcuts {
			ox := x + mmf(off.X)
			oy := flip(mmf(off.Y), mmf(off.H))
			rect(ox, oy, mmf(off.W), mmf(off.H), "OFFCUT")
		}

		x += sheetW + gapMM
		if top := sheetH + 50; top > maxY {
			maxY = top
		}
		maxX = x - gapMM
	}

	b.WriteString("0\nENDSEC\n0\nEOF\n")

	// Header written last because the extents are only known after the layout.
	var out strings.Builder
	out.WriteString("0\nSECTION\n2\nHEADER\n")
	writeHeaderVar(&out, "$ACADVER", "AC1009")
	writeHeaderInt(&out, "$INSUNITS", 4) // 4 = millimeters
	writeHeaderPoint(&out, "$EXTMIN", 0, 0)
	writeHeaderPoint(&out, "$EXTMAX", maxX, maxY)
	out.WriteString("0\nENDSEC\n")
	out.WriteString(b.String())

	_, err := io.WriteString(w, out.String())
	return err
}

// writeLine writes a LINE entity in millimetres.
func writeLine(b *strings.Builder, layer string, x1, y1, x2, y2 float64) {
	fmt.Fprintf(b, "0\nLINE\n8\n%s\n10\n%.3f\n20\n%.3f\n30\n0.0\n11\n%.3f\n21\n%.3f\n31\n0.0\n",
		layer, x1, y1, x2, y2)
}

func writeText(b *strings.Builder, layer string, x, y, height float64, text string) {
	fmt.Fprintf(b, "0\nTEXT\n8\n%s\n10\n%.3f\n20\n%.3f\n30\n0.0\n40\n%.2f\n1\n%s\n",
		layer, x, y, height, sanitizeText(text))
}

func writeHeaderVar(b *strings.Builder, name, value string) {
	fmt.Fprintf(b, "9\n%s\n1\n%s\n", name, value)
}

func writeHeaderInt(b *strings.Builder, name string, value int) {
	fmt.Fprintf(b, "9\n%s\n70\n%d\n", name, value)
}

func writeHeaderPoint(b *strings.Builder, name string, x, y float64) {
	fmt.Fprintf(b, "9\n%s\n10\n%.3f\n20\n%.3f\n30\n0.0\n", name, x, y)
}

// sanitizeText keeps DXF text on one line and ASCII-clean.
func sanitizeText(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	var b strings.Builder
	for _, r := range s {
		if r < 32 || r > 126 {
			b.WriteRune('?')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
