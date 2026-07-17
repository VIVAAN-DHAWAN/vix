package pdf

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// Document is a parsed PDF file: its cross-reference table, trailer and a cache
// of resolved indirect objects.
type Document struct {
	buf     []byte
	xref    map[int]xrefEntry
	trailer Dict
	cache   map[int]Object
	stmObjs map[int][]Object // decoded object-stream contents, keyed by stream obj num
}

type xrefEntry struct {
	typ    int   // 1 = uncompressed (byte offset), 2 = compressed (in object stream)
	offset int64 // typ 1: file offset; typ 2: containing object-stream number
	index  int   // typ 2: index within the object stream
}

// ErrEncrypted is returned when the PDF declares an /Encrypt dictionary; v1 does
// not attempt decryption.
var ErrEncrypted = errors.New("pdf: encrypted document not supported")

// Parse reads a PDF file's structure from raw bytes.
func Parse(data []byte) (*Document, error) {
	if !bytes.HasPrefix(bytes.TrimLeft(data[:min(16, len(data))], "\x00 \t\r\n"), []byte("%PDF-")) {
		// Some files have leading junk; locate the header within the first 1KB.
		if idx := bytes.Index(data[:min(1024, len(data))], []byte("%PDF-")); idx > 0 {
			data = data[idx:]
		} else {
			return nil, fmt.Errorf("pdf: not a PDF (missing %%PDF- header)")
		}
	}
	doc := &Document{
		buf:     data,
		xref:    map[int]xrefEntry{},
		cache:   map[int]Object{},
		stmObjs: map[int][]Object{},
	}
	if err := doc.readXref(); err != nil || len(doc.xref) == 0 {
		doc.rebuildXref()
	}
	if _, ok := doc.trailer["Encrypt"]; ok {
		return nil, ErrEncrypted
	}
	// If the cross-reference table did not yield a resolvable Catalog (a common
	// symptom of misaligned offsets), rebuild it by scanning object headers.
	if !doc.hasCatalog() {
		doc.rebuildXref()
		doc.cache = map[int]Object{}
		if doc.trailer == nil || doc.trailer["Root"] == nil || !doc.hasCatalog() {
			if doc.trailer == nil {
				doc.trailer = Dict{}
			}
			doc.rebuildTrailer()
		}
	}
	return doc, nil
}

var startxrefRe = regexp.MustCompile(`startxref\s+(\d+)`)

// readXref parses the cross-reference chain starting at the last startxref.
func (d *Document) readXref() error {
	tail := d.buf
	if len(tail) > 2048 {
		tail = tail[len(tail)-2048:]
	}
	m := startxrefRe.FindAllSubmatch(tail, -1)
	if m == nil {
		return fmt.Errorf("pdf: no startxref")
	}
	off, _ := strconv.Atoi(string(m[len(m)-1][1]))
	seen := map[int]bool{}
	for off > 0 && off < len(d.buf) && !seen[off] {
		seen[off] = true
		next, hybrid, err := d.readXrefSection(off)
		if err != nil {
			return err
		}
		// Hybrid-reference files point to an xref stream via /XRefStm.
		if hybrid > 0 && !seen[hybrid] {
			seen[hybrid] = true
			d.readXrefSection(hybrid)
		}
		off = next
	}
	return nil
}

// readXrefSection parses either a classic xref table or an xref stream at off.
// It returns the /Prev offset (0 if none) and any /XRefStm hybrid offset.
func (d *Document) readXrefSection(off int) (prev int, hybrid int, err error) {
	p := newObjParser(d.buf, d)
	p.lex.pos = off
	p.lex.skipWhite()
	if bytes.HasPrefix(d.buf[p.lex.pos:], []byte("xref")) {
		return d.readClassicXref(off)
	}
	return d.readXrefStream(off)
}

var xrefSubRe = regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s*$`)

func (d *Document) readClassicXref(off int) (prev int, hybrid int, err error) {
	buf := d.buf
	i := off + len("xref")
	for i < len(buf) {
		// Skip EOL/space.
		for i < len(buf) && isWhite(buf[i]) {
			i++
		}
		if bytes.HasPrefix(buf[i:], []byte("trailer")) {
			i += len("trailer")
			break
		}
		// Subsection header: "start count".
		lineEnd := i
		for lineEnd < len(buf) && buf[lineEnd] != '\n' && buf[lineEnd] != '\r' {
			lineEnd++
		}
		mm := xrefSubRe.FindSubmatch(buf[i:lineEnd])
		if mm == nil {
			break
		}
		start, _ := strconv.Atoi(string(mm[1]))
		count, _ := strconv.Atoi(string(mm[2]))
		i = lineEnd
		for i < len(buf) && isWhite(buf[i]) {
			i++
		}
		// Each entry is 20 bytes: "nnnnnnnnnn ggggg t\r\n".
		for k := 0; k < count && i+18 <= len(buf); k++ {
			entry := buf[i : i+18]
			offset, _ := strconv.Atoi(string(bytes.TrimSpace(entry[0:10])))
			typ := entry[17]
			num := start + k
			if typ == 'n' {
				if _, exists := d.xref[num]; !exists {
					d.xref[num] = xrefEntry{typ: 1, offset: int64(offset)}
				}
			}
			i += 20
			// Tolerate 19-byte entries (some producers use a single EOL char).
			if i-2 < len(buf) && !isWhite(buf[i-2]) {
				i--
			}
		}
	}
	// Parse trailer dictionary.
	p := newObjParser(buf, d)
	p.lex.pos = i
	obj, perr := p.parseObject()
	if perr == nil {
		if td, ok := obj.(Dict); ok {
			if d.trailer == nil {
				d.trailer = td
			}
			if n, ok := Int(td["Prev"]); ok {
				prev = n
			}
			if n, ok := Int(td["XRefStm"]); ok {
				hybrid = n
			}
		}
	}
	return prev, hybrid, nil
}

func (d *Document) readXrefStream(off int) (prev int, hybrid int, err error) {
	p := newObjParser(d.buf, d)
	p.lex.pos = off
	obj, perr := p.parseObject()
	if perr != nil {
		return 0, 0, perr
	}
	st, ok := obj.(Stream)
	if !ok {
		return 0, 0, fmt.Errorf("pdf: xref stream expected at %d", off)
	}
	data, derr := d.decodeStream(st)
	if derr != nil {
		return 0, 0, derr
	}
	dct := st.Dict
	if d.trailer == nil {
		d.trailer = dct
	}
	// /W widths of the three fields.
	wArr, _ := dct["W"].(Array)
	if len(wArr) < 3 {
		return 0, 0, fmt.Errorf("pdf: bad xref stream /W")
	}
	w0, _ := Int(wArr[0])
	w1, _ := Int(wArr[1])
	w2, _ := Int(wArr[2])
	rowLen := w0 + w1 + w2
	if rowLen == 0 {
		return 0, 0, fmt.Errorf("pdf: zero-width xref stream")
	}
	// Index pairs (default [0 Size]).
	var index []int
	if idxArr, ok := dct["Index"].(Array); ok {
		for _, v := range idxArr {
			if n, ok := Int(v); ok {
				index = append(index, n)
			}
		}
	} else {
		size, _ := Int(dct["Size"])
		index = []int{0, size}
	}
	readField := func(b []byte, width int) int64 {
		var v int64
		for _, c := range b[:width] {
			v = v<<8 | int64(c)
		}
		return v
	}
	pos := 0
	for pair := 0; pair+1 < len(index); pair += 2 {
		start := index[pair]
		count := index[pair+1]
		for k := 0; k < count && pos+rowLen <= len(data); k++ {
			row := data[pos : pos+rowLen]
			pos += rowLen
			f0 := int64(1)
			if w0 > 0 {
				f0 = readField(row, w0)
			}
			f1 := readField(row[w0:], w1)
			f2 := readField(row[w0+w1:], w2)
			num := start + k
			if _, exists := d.xref[num]; exists {
				continue
			}
			switch f0 {
			case 1:
				d.xref[num] = xrefEntry{typ: 1, offset: f1}
			case 2:
				d.xref[num] = xrefEntry{typ: 2, offset: f1, index: int(f2)}
			}
		}
	}
	if n, ok := Int(dct["Prev"]); ok {
		prev = n
	}
	return prev, 0, nil
}

var objHeaderRe = regexp.MustCompile(`(?m)(\d+)\s+(\d+)\s+obj\b`)

// rebuildXref brute-force scans the file for "n g obj" definitions when the
// cross-reference table is missing or corrupt.
func (d *Document) rebuildXref() {
	for _, m := range objHeaderRe.FindAllSubmatchIndex(d.buf, -1) {
		num, _ := strconv.Atoi(string(d.buf[m[2]:m[3]]))
		// Last definition wins (later objects supersede earlier in a scan).
		d.xref[num] = xrefEntry{typ: 1, offset: int64(m[0])}
	}
}

// hasCatalog reports whether the trailer's /Root resolves to a Catalog dict.
func (d *Document) hasCatalog() bool {
	if d.trailer == nil {
		return false
	}
	cat, ok := d.Resolve(d.trailer["Root"]).(Dict)
	if !ok {
		return false
	}
	if name(cat["Type"]) == "Catalog" {
		return true
	}
	// Some producers omit /Type on the catalog; accept a /Pages pointer.
	_, hasPages := d.Resolve(cat["Pages"]).(Dict)
	return hasPages
}

// rebuildTrailer finds a trailer dict (or a /Root Catalog) when none was parsed.
func (d *Document) rebuildTrailer() {
	// Look for any object with /Type /Catalog and synthesize a trailer.
	for num := range d.xref {
		obj := d.Resolve(Reference{Num: num})
		if dct, ok := obj.(Dict); ok {
			if name(dct["Type"]) == "Catalog" {
				d.trailer["Root"] = Reference{Num: num}
				return
			}
		}
	}
}

// Resolve dereferences an indirect reference (recursively for chained refs),
// returning the concrete object. Non-references are returned as-is.
func (d *Document) Resolve(o Object) Object {
	for depth := 0; depth < 32; depth++ {
		ref, ok := o.(Reference)
		if !ok {
			return o
		}
		if cached, ok := d.cache[ref.Num]; ok {
			o = cached
			continue
		}
		obj := d.loadObject(ref.Num)
		d.cache[ref.Num] = obj
		o = obj
	}
	return o
}

func (d *Document) loadObject(num int) Object {
	e, ok := d.xref[num]
	if !ok {
		return Null{}
	}
	if e.typ == 2 {
		return d.loadFromObjStm(int(e.offset), e.index)
	}
	if e.offset <= 0 || int(e.offset) >= len(d.buf) {
		return Null{}
	}
	p := newObjParser(d.buf, d)
	p.lex.pos = int(e.offset)
	obj, err := p.parseObject()
	if err != nil {
		return Null{}
	}
	return obj
}

// loadFromObjStm returns the object at index within object stream stmNum.
func (d *Document) loadFromObjStm(stmNum, index int) Object {
	objs, ok := d.stmObjs[stmNum]
	if !ok {
		objs = d.parseObjStm(stmNum)
		d.stmObjs[stmNum] = objs
	}
	if index >= 0 && index < len(objs) {
		return objs[index]
	}
	return Null{}
}

func (d *Document) parseObjStm(stmNum int) []Object {
	e, ok := d.xref[stmNum]
	if !ok || e.typ != 1 {
		return nil
	}
	p := newObjParser(d.buf, d)
	p.lex.pos = int(e.offset)
	obj, err := p.parseObject()
	if err != nil {
		return nil
	}
	st, ok := obj.(Stream)
	if !ok {
		return nil
	}
	data, err := d.decodeStream(st)
	if err != nil {
		return nil
	}
	n, _ := Int(st.Dict["N"])
	first, _ := Int(st.Dict["First"])
	// Header: N pairs of "objNum offset".
	hp := newObjParser(data, d)
	offsets := make([]int, 0, n)
	for k := 0; k < n; k++ {
		if _, err := hp.lex.next(); err != nil { // objNum (ignored)
			break
		}
		ot, err := hp.lex.next()
		if err != nil || ot.kind != tokInt {
			break
		}
		offsets = append(offsets, int(ot.ival))
	}
	objs := make([]Object, len(offsets))
	for k, o := range offsets {
		start := first + o
		if start < 0 || start > len(data) {
			continue
		}
		op := newObjParser(data, d)
		op.lex.pos = start
		if val, err := op.parseObject(); err == nil {
			objs[k] = val
		}
	}
	return objs
}

// decodeStream applies the stream's filter pipeline and returns the decoded
// bytes.
func (d *Document) decodeStream(s Stream) ([]byte, error) {
	data := s.Raw
	filters := d.filterList(s.Dict["Filter"])
	parms := d.parmsList(s.Dict["DecodeParms"], len(filters))
	for i, f := range filters {
		var err error
		switch f {
		case "FlateDecode", "Fl":
			data, err = flateDecode(data)
			if err == nil {
				data, err = d.maybePredictor(data, parms[i])
			}
		case "LZWDecode", "LZW":
			ec := 1
			if p := parms[i]; p != nil {
				if v, ok := Int(d.Resolve(p["EarlyChange"])); ok {
					ec = v
				}
			}
			data, err = lzwDecode(data, ec)
			if err == nil {
				data, err = d.maybePredictor(data, parms[i])
			}
		case "ASCIIHexDecode", "AHx":
			data, err = asciiHexDecode(data)
		case "ASCII85Decode", "A85":
			data, err = ascii85Decode(data)
		case "RunLengthDecode", "RL":
			data, err = runLengthDecode(data)
		case "DCTDecode", "JPXDecode", "CCITTFaxDecode", "JBIG2Decode":
			// Image filters: no text to extract.
			return nil, fmt.Errorf("pdf: image filter %s", f)
		default:
			// Unknown filter: pass through.
		}
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func (d *Document) maybePredictor(data []byte, parms Dict) ([]byte, error) {
	if parms == nil {
		return data, nil
	}
	pred, _ := Int(d.Resolve(parms["Predictor"]))
	if pred < 2 {
		return data, nil
	}
	colors, _ := Int(d.Resolve(parms["Colors"]))
	bpc, _ := Int(d.Resolve(parms["BitsPerComponent"]))
	columns, _ := Int(d.Resolve(parms["Columns"]))
	return applyPredictor(data, pred, colors, bpc, columns)
}

func (d *Document) filterList(o Object) []Name {
	o = d.Resolve(o)
	switch v := o.(type) {
	case Name:
		return []Name{v}
	case Array:
		var out []Name
		for _, e := range v {
			if n, ok := d.Resolve(e).(Name); ok {
				out = append(out, n)
			}
		}
		return out
	}
	return nil
}

func (d *Document) parmsList(o Object, n int) []Dict {
	out := make([]Dict, n)
	o = d.Resolve(o)
	switch v := o.(type) {
	case Dict:
		if n > 0 {
			out[0] = v
		}
	case Array:
		for i := 0; i < n && i < len(v); i++ {
			if dct, ok := d.Resolve(v[i]).(Dict); ok {
				out[i] = dct
			}
		}
	}
	return out
}
