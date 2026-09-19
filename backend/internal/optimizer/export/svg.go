package export

import (
	"fmt"
	"io"
	"strings"

	"github.com/size-module/backend/internal/optimizer/core"
)

// svgPalette gives every part code a stable colour, like the viewer does.
func svgHue(code string) int {
	h := uint32(2166136261)
	for i := 0; i < len(code); i++ {
		h ^= uint32(code[i])
		h *= 16777619
	}
	return int(h % 360)
}

// SVG writes one drawing with every sheet stacked vertically: sheet outline,
// trim line, pieces with their code and size, and reusable offcuts in green.
// Coordinates are millimetres; the drawing is scaled to a readable pixel size.
func SVG(w io.Writer, sol core.Solution, opts Options) error {
	sheets := sheetsOf(sol, opts)
	if len(sheets) == 0 {
		return fmt.Errorf("no sheets to export")
	}

	// Scale the widest sheet into a readable strip.
	const (
		pad      = 24.0
		targetPx = 1100.0
		maxScale = 0.5  // px per mm
		minScale = 0.02 // px per mm
	)
	maxW := 0.0
	for _, sheet := range sheets {
		if wmm := mmf(sheet.Width); wmm > maxW {
			maxW = wmm
		}
	}
	scale := targetPx / maxW
	if scale > maxScale {
		scale = maxScale
	}
	if scale < minScale {
		scale = minScale
	}

	// First pass: total height.
	height := pad + 34 // header
	for _, sheet := range sheets {
		height += 20 + mmf(sheet.Height)*scale + 18
	}
	height += pad
	width := pad*2 + maxW*scale

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family="Helvetica, Arial, sans-serif">`,
		width, height, width, height)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#ffffff"/>`)
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-size="14" font-weight="bold">%s</text>`,
		pad, pad+6, xmlEscape("Plan — "+sol.Solver+" "+sol.SolverVersion))
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-size="11" fill="#555">%s</text>`,
		pad, pad+22, xmlEscape(fmt.Sprintf(
			"%d sheet(s) · %d/%d pieces · yield %.1f%% · waste %.1f%%",
			sol.Metrics.SheetCount, sol.Metrics.PartsPlaced, sol.Metrics.PartsRequested,
			sol.Metrics.YieldPct, sol.Metrics.WastePct)))

	y := pad + 34
	for _, sheet := range sheets {
		sw := mmf(sheet.Width) * scale
		sh := mmf(sheet.Height) * scale
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="11" font-weight="bold">%s</text>`,
			pad, y+12, xmlEscape(fmt.Sprintf("Sheet %d · %s · %s × %s mm",
				sheet.Index+1, sheet.Label, mm(sheet.Width), mm(sheet.Height))))
		y += 20

		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#fafafa" stroke="#333" stroke-width="1"/>`,
			pad, y, sw, sh)

		for _, pl := range sheet.Placements {
			x := pad + mmf(pl.X)*scale
			py := y + mmf(pl.Y)*scale
			pw := mmf(pl.W) * scale
			ph := mmf(pl.H) * scale
			hue := svgHue(pl.PartCode)
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="hsl(%d,65%%,80%%)" stroke="hsl(%d,55%%,35%%)" stroke-width="0.8"/>`,
				x, py, pw, ph, hue, hue)
			if pw > 40 && ph > 14 {
				fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="9" text-anchor="middle">%s</text>`,
					x+pw/2, py+ph/2-2, xmlEscape(truncate(pl.PartCode, 22)))
				fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="8" fill="#444" text-anchor="middle">%s</text>`,
					x+pw/2, py+ph/2+9, xmlEscape(mm(pl.W)+" × "+mm(pl.H)))
			}
		}
		for _, off := range sheet.Offcuts {
			x := pad + mmf(off.X)*scale
			oy := y + mmf(off.Y)*scale
			ow := mmf(off.W) * scale
			oh := mmf(off.H) * scale
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#d1fae5" fill-opacity="0.7" stroke="#059669" stroke-width="0.8" stroke-dasharray="4 3"/>`,
				x, oy, ow, oh)
			if ow > 60 && oh > 16 {
				fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="8" fill="#065f46">%s</text>`,
					x+3, oy+11, xmlEscape("offcut "+mm(off.W)+" × "+mm(off.H)))
			}
		}
		y += sh + 18
	}
	b.WriteString(`</svg>`)
	_, err := io.WriteString(w, b.String())
	return err
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}
