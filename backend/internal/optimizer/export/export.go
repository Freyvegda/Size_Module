// Package export writes cutting plans in the formats shops actually use: CSV
// cut lists for the operator, SVG drawings for review, DXF for CAD/CAM and a
// printable PDF. Everything is pure Go with no external dependencies and works
// on a core.Solution, so exports are identical to what the viewer shows.
package export

import (
	"io"

	"github.com/size-module/backend/internal/optimizer/core"
)

// Format identifies a supported export format.
type Format string

const (
	FormatCSV Format = "csv"
	FormatSVG Format = "svg"
	FormatDXF Format = "dxf"
	FormatPDF Format = "pdf"
)

// Supported reports whether a format name is known.
func Supported(format string) bool {
	switch Format(format) {
	case FormatCSV, FormatSVG, FormatDXF, FormatPDF:
		return true
	default:
		return false
	}
}

// ContentType returns the MIME type of a format.
func ContentType(format Format) string {
	switch format {
	case FormatCSV:
		return "text/csv; charset=utf-8"
	case FormatSVG:
		return "image/svg+xml"
	case FormatDXF:
		return "application/dxf"
	case FormatPDF:
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

// Extension returns the file extension (without the dot) of a format.
func Extension(format Format) string { return string(format) }

// Options narrows an export.
type Options struct {
	// Sheet, when set, exports only that sheet index.
	Sheet *int
}

// Write renders the solution in the named format. It returns the format that
// was written so callers can set headers in one place.
func Write(w io.Writer, format Format, sol core.Solution, opts Options) error {
	switch format {
	case FormatCSV:
		return CSV(w, sol, opts)
	case FormatSVG:
		return SVG(w, sol, opts)
	case FormatDXF:
		return DXF(w, sol, opts)
	case FormatPDF:
		return PDF(w, sol, opts)
	default:
		return errUnknownFormat(string(format))
	}
}

type errUnknownFormat string

func (e errUnknownFormat) Error() string { return "unknown export format " + string(e) }

// sheetsOf returns the sheets an export covers, honouring Options.Sheet.
func sheetsOf(sol core.Solution, opts Options) []core.SheetPlan {
	if opts.Sheet == nil {
		return sol.Sheets
	}
	for _, sheet := range sol.Sheets {
		if sheet.Index == *opts.Sheet {
			return []core.SheetPlan{sheet}
		}
	}
	return nil
}

// mmf returns a micrometer length in millimeters as a float.
func mmf(d core.Dim) float64 { return float64(d) / float64(core.Millimeter) }

// mm formats a micrometer length as millimeters with enough precision for a
// shop drawing (0.01 mm).
func mm(d core.Dim) string {
	negative := d < 0
	if negative {
		d = -d
	}
	whole := d / core.Millimeter
	hundredths := (d % core.Millimeter) / 10
	wholeStr := itoa(whole)
	if hundredths == 0 {
		if negative {
			return "-" + wholeStr
		}
		return wholeStr
	}
	frac := itoa(hundredths)
	if hundredths < 10 {
		frac = "0" + frac
	}
	if negative {
		return "-" + wholeStr + "." + frac
	}
	return wholeStr + "." + frac
}

func itoa(v core.Dim) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
