package daemon

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/get-vix/vix/internal/config"
	"github.com/get-vix/vix/internal/daemon/pdf"
)

// maxPDFBytes caps the size of a PDF that read_file will attempt to convert, to
// bound parse time and memory. Larger files return a clear error.
const maxPDFBytes = 50 << 20 // 50 MiB

// looksLikePDF reports whether raw is a PDF by locating the %PDF- header within
// the first kilobyte (some files carry a small amount of leading junk).
func looksLikePDF(raw []byte) bool {
	head := raw
	if len(head) > 1024 {
		head = head[:1024]
	}
	return bytes.Contains(head, []byte("%PDF-"))
}

// pdfToMarkdown converts a PDF's bytes to Markdown for read_file, prefixed with
// a provenance comment. It returns actionable messages for encrypted, scanned,
// or oversized documents rather than raw errors.
func pdfToMarkdown(path string, raw []byte) (string, error) {
	if len(raw) > maxPDFBytes {
		return "", fmt.Errorf("PDF is too large to convert (%d bytes, limit %d). Extract a page range with an external tool, or read it via bash.", len(raw), maxPDFBytes)
	}
	res, err := pdf.ToMarkdown(raw)
	if err != nil {
		if err == pdf.ErrEncrypted {
			return "", fmt.Errorf("this PDF is encrypted; vix cannot extract its text")
		}
		return "", fmt.Errorf("could not parse PDF: %v", err)
	}
	base := filepath.Base(path)
	if res.Scanned {
		return fmt.Sprintf("<!-- %s: %d page(s), no extractable text -->\n\nThis PDF appears to be scanned or image-only; it has no text layer to extract. OCR is not supported.",
			base, res.Pages), nil
	}
	header := fmt.Sprintf("<!-- converted from %s · %d page(s) · vix pdf reader -->\n\n", base, res.Pages)
	return header + res.Markdown, nil
}

// pdfEnabled reports whether the PDF-to-Markdown path is active.
func pdfEnabled() bool { return config.PDFEnabled() }
