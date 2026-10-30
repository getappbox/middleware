package middleware

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"unicode/utf16"

	"howett.net/plist"
)

const maxPlistDepth = 128
const maxPlistNodes = 100_000
const maxPlistScalarBytes = 16 * 1024 * 1024

var utf16XMLDeclaration = regexp.MustCompile(`(?i)^(\s*<\?xml\b[^?]*\bencoding\s*=\s*["'])utf-16(?:le|be)?(["'][^?]*\?>)`)

func decodeManifest(body []byte, manifest *ipaManifest) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	if !boundedPlist(body) {
		return false
	}
	if len(body) >= 2 && (body[0] == 0xfe && body[1] == 0xff || body[0] == 0xff && body[1] == 0xfe || body[0] == 0 && body[1] != 0 || body[0] != 0 && body[1] == 0) {
		text, _ := plistText(body)
		body = []byte(text)
	}
	_, err := plist.Unmarshal(body, manifest)
	return err == nil
}

func boundedPlist(body []byte) bool {
	if bytes.HasPrefix(body, []byte("bplist")) {
		return boundedBinaryPlist(body)
	}
	text, ok := plistText(body)
	if !ok || len(text) > maxPlistScalarBytes {
		return false
	}
	looksXML := strings.HasPrefix(strings.TrimSpace(text), "<")
	depth, nodes := 0, 0
	decoder := xml.NewDecoder(strings.NewReader(text))
	for {
		token, err := decoder.Token()
		if err != nil {
			if looksXML && nodes > 0 {
				return err == io.EOF && depth == 0
			}
			if looksXML && (strings.HasPrefix(strings.TrimSpace(text), "<?xml") || strings.HasPrefix(strings.TrimSpace(text), "<!DOCTYPE")) {
				return false
			}
			break
		}
		switch token.(type) {
		case xml.StartElement:
			depth++
			nodes++
			if depth > maxPlistDepth || nodes > maxPlistNodes {
				return false
			}
		case xml.EndElement:
			depth--
		}
	}
	return boundedTextPlist(text)
}

func plistText(body []byte) (string, bool) {
	if bytes.HasPrefix(body, []byte{0xef, 0xbb, 0xbf}) {
		return string(body[3:]), true
	}
	var order binary.ByteOrder
	if len(body) >= 2 {
		switch {
		case body[0] == 0xfe && body[1] == 0xff:
			order, body = binary.BigEndian, body[2:]
		case body[0] == 0xff && body[1] == 0xfe:
			order, body = binary.LittleEndian, body[2:]
		case body[0] == 0 && body[1] != 0:
			order = binary.BigEndian
		case body[0] != 0 && body[1] == 0:
			order = binary.LittleEndian
		}
	}
	if order == nil {
		return string(body), true
	}
	if len(body)%2 != 0 {
		return "", false
	}
	units := make([]uint16, len(body)/2)
	for i := range units {
		units[i] = order.Uint16(body[i*2:])
	}
	text := string(utf16.Decode(units))
	return utf16XMLDeclaration.ReplaceAllString(text, "${1}UTF-8${2}"), true
}

func boundedTextPlist(text string) bool {
	depth, nodes := 0, 0
	for index := 0; index < len(text); {
		switch text[index] {
		case ' ', '\t', '\r', '\n', '\v', '\f', ';', '=', ',':
			index++
		case '"':
			nodes++
			index++
			closed := false
			for index < len(text) {
				if text[index] == '\\' {
					index += 2
					continue
				}
				if text[index] == '"' {
					index++
					closed = true
					break
				}
				index++
			}
			if !closed {
				return false
			}
		case '{', '(':
			depth++
			nodes++
			index++
			if depth > maxPlistDepth {
				return false
			}
		case '}', ')':
			depth--
			index++
			if depth < 0 {
				return false
			}
		case '<':
			nodes++
			end := strings.IndexByte(text[index:], '>')
			if end < 0 {
				return false
			}
			index += end + 1
		default:
			if strings.HasPrefix(text[index:], "//") {
				end := strings.IndexAny(text[index:], "\r\n")
				if end < 0 {
					return depth == 0
				}
				index += end
				continue
			}
			if strings.HasPrefix(text[index:], "/*") {
				end := strings.Index(text[index+2:], "*/")
				if end < 0 {
					return false
				}
				index += end + 4
				continue
			}
			nodes++
			for index < len(text) && !strings.ContainsRune(" \t\r\n\v\f{}()=;,\"<>", rune(text[index])) {
				index++
			}
			if index < len(text) && text[index] == '>' {
				index++
			}
		}
		if nodes > maxPlistNodes {
			return false
		}
	}
	return depth == 0
}

func binaryUint(body []byte, offset, width uint64) (uint64, bool) {
	if width == 0 || width > 8 || offset > uint64(len(body)) || width > uint64(len(body))-offset {
		return 0, false
	}
	var value uint64
	for _, part := range body[offset : offset+width] {
		value = value<<8 | uint64(part)
	}
	return value, true
}

func boundedBinaryPlist(body []byte) bool {
	if len(body) < 42 || !bytes.HasPrefix(body, []byte("bplist00")) && !bytes.HasPrefix(body, []byte("bplist01")) {
		return false
	}
	trailer := uint64(len(body) - 32)
	offsetWidth, refWidth := uint64(body[trailer+6]), uint64(body[trailer+7])
	count := binary.BigEndian.Uint64(body[trailer+8:])
	top := binary.BigEndian.Uint64(body[trailer+16:])
	table := binary.BigEndian.Uint64(body[trailer+24:])
	if offsetWidth < 1 || offsetWidth > 8 || refWidth < 1 || refWidth > 8 || count == 0 || count > maxPlistNodes || top >= count || table < 9 || table >= trailer || count > (trailer-table)/offsetWidth || count*offsetWidth != trailer-table {
		return false
	}
	if refWidth < 8 && count > uint64(1)<<(8*refWidth) {
		return false
	}
	offsets := make([]uint64, int(count))
	for index := range offsets {
		offset, ok := binaryUint(body, table+uint64(index)*offsetWidth, offsetWidth)
		if !ok || offset < 8 || offset >= table {
			return false
		}
		offsets[index] = offset
	}
	states := make([]byte, int(count))
	costs, heights := make([]int, int(count)), make([]int, int(count))
	scalarBytes := uint64(0)
	var visit func(uint64, int) bool
	visit = func(object uint64, depth int) bool {
		if object >= count || depth > maxPlistDepth || states[object] == 1 {
			return false
		}
		if states[object] == 2 {
			return depth+heights[object]-1 <= maxPlistDepth
		}
		states[object] = 1
		offset := offsets[object]
		tag, start := body[offset], offset+1
		size := uint64(tag & 0x0f)
		kind := tag & 0xf0
		if kind == 0x40 || kind == 0x50 || kind == 0x60 || kind == 0xa0 || kind == 0xd0 {
			if size == 15 {
				if start >= table || body[start]&0xf0 != 0x10 || body[start]&0x0f > 3 {
					return false
				}
				width := uint64(1) << (body[start] & 0x0f)
				if width > table-start-1 {
					return false
				}
				var ok bool
				size, ok = binaryUint(body, start+1, width)
				if !ok {
					return false
				}
				start += width + 1
			}
		}
		cost, height := 1, 1
		switch kind {
		case 0x00:
		case 0x10, 0x20:
			if size > 4 || uint64(1)<<size > table-start {
				return false
			}
		case 0x30:
			if 8 > table-start {
				return false
			}
		case 0x40, 0x50:
			if size > table-start {
				return false
			}
			if size > maxPlistScalarBytes-scalarBytes {
				return false
			}
			scalarBytes += size
		case 0x60:
			if size > (table-start)/2 {
				return false
			}
			if size > (maxPlistScalarBytes-scalarBytes)/3 {
				return false
			}
			scalarBytes += size * 3
		case 0x80:
			if size+1 > table-start {
				return false
			}
		case 0xa0, 0xd0:
			multiplier := uint64(1)
			if kind == 0xd0 {
				multiplier = 2
			}
			if size > maxPlistNodes || size > (table-start)/refWidth/multiplier {
				return false
			}
			for index := uint64(0); index < size*multiplier; index++ {
				child, ok := binaryUint(body, start+index*refWidth, refWidth)
				if !ok || !visit(child, depth+1) {
					return false
				}
				cost += costs[child]
				height = max(height, heights[child]+1)
				if cost > maxPlistNodes || height > maxPlistDepth {
					return false
				}
			}
		default:
			return false
		}
		states[object], costs[object], heights[object] = 2, cost, height
		return true
	}
	return visit(top, 1)
}
