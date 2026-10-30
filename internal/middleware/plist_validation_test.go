package middleware

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"

	"howett.net/plist"
)

func plistWithObjects(objects []byte, offsets []uint64, refWidth byte) []byte {
	body := append([]byte("bplist00"), objects...)
	table := uint64(len(body))
	for _, offset := range offsets {
		var raw [4]byte
		binary.BigEndian.PutUint32(raw[:], uint32(offset))
		body = append(body, raw[:]...)
	}
	trailer := make([]byte, 32)
	trailer[6], trailer[7] = 4, refWidth
	binary.BigEndian.PutUint64(trailer[8:], uint64(len(offsets)))
	binary.BigEndian.PutUint64(trailer[24:], table)
	return append(body, trailer...)
}

func TestManifestPlistBoundsPreserveSupportedFormats(t *testing.T) {
	for _, format := range []int{plist.XMLFormat, plist.BinaryFormat} {
		body, err := plist.Marshal(manifestValue(), format)
		if err != nil {
			t.Fatal(err)
		}
		if !boundedPlist(body) {
			t.Fatal("valid format rejected", format)
		}
		if _, ok := updatedManifest(body); !ok {
			t.Fatal("valid manifest not rewritten", format)
		}
	}
	openStep := `{items=({assets=({kind="software-package";url="https://dl.dropboxusercontent.com/s/a/app.ipa";});});}`
	commented := "/* " + strings.Repeat("{()}", 1000) + " */\n" + openStep + " // trailing comment"
	for _, text := range []string{openStep, commented} {
		if !boundedPlist([]byte(text)) {
			t.Fatal("valid OpenStep rejected")
		}
		if _, ok := updatedManifest([]byte(text)); !ok {
			t.Fatal("valid OpenStep not rewritten")
		}
	}
	units := utf16.Encode([]rune(openStep))
	utf16Body := []byte{0xff, 0xfe}
	for _, unit := range units {
		utf16Body = append(utf16Body, byte(unit), byte(unit>>8))
	}
	if !boundedPlist(utf16Body) {
		t.Fatal("valid UTF16 OpenStep rejected")
	}
	if _, ok := updatedManifest(utf16Body); !ok {
		t.Fatal("valid UTF16 OpenStep not rewritten")
	}
	xmlBody, _ := plist.Marshal(manifestValue(), plist.XMLFormat)
	xmlText := strings.Replace(string(xmlBody), `encoding="UTF-8"`, `encoding="UTF-16"`, 1)
	if _, ok := updatedManifest(encodeUTF16XML(xmlText)); !ok {
		t.Fatal("valid UTF16 XML not rewritten")
	}
}

func encodeUTF16XML(text string) []byte {
	body := []byte{0xff, 0xfe}
	for _, unit := range utf16.Encode([]rune(text)) {
		body = append(body, byte(unit), byte(unit>>8))
	}
	return body
}

func TestMalformedBinaryManifestsDoNotPanicOrAllocateFromClaims(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"UTF16 length overflow", plistWithObjects([]byte{0x6f, 0x13, 0x80, 0, 0, 0, 0, 0, 0, 0}, []uint64{8}, 1)},
		{"ASCII length overflow", plistWithObjects([]byte{0x5f, 0x13, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xf0}, []uint64{8}, 1)},
		{"zero reference width", plistWithObjects([]byte{0xaf, 0x13, 0x80, 0, 0, 0, 0, 0, 0, 0}, []uint64{8}, 0)},
		{"array length overflow", plistWithObjects([]byte{0xaf, 0x13, 0x80, 0, 0, 0, 0, 0, 0, 0}, []uint64{8}, 2)},
		{"dict length overflow", plistWithObjects([]byte{0xdf, 0x13, 0x80, 0, 0, 0, 0, 0, 0, 0}, []uint64{8}, 1)},
		{"out of range reference", plistWithObjects([]byte{0xa1, 2}, []uint64{8}, 1)},
		{"reference cycle", plistWithObjects([]byte{0xa1, 0}, []uint64{8}, 1)},
		{"truncated numeric payload", plistWithObjects([]byte{0x13}, []uint64{8}, 1)},
		{"invalid object offset", plistWithObjects([]byte{0x09}, []uint64{0}, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if boundedPlist(tc.body) {
				t.Fatal("malformed plist accepted")
			}
			if _, ok := updatedManifest(tc.body); ok {
				t.Fatal("malformed plist rewritten")
			}
		})
	}
	valid, _ := plist.Marshal(manifestValue(), plist.BinaryFormat)
	for index := range valid {
		if _, ok := updatedManifest(valid[:index]); ok {
			t.Fatalf("truncated plist %d rewritten", index)
		}
	}
}

func TestPlistNestingAndExpansionBudgets(t *testing.T) {
	deepXML := []byte(`<plist>` + strings.Repeat(`<array>`, maxPlistDepth) + `<string>x</string>` + strings.Repeat(`</array>`, maxPlistDepth) + `</plist>`)
	deepText := []byte(strings.Repeat("(", maxPlistDepth+1) + `"x"` + strings.Repeat(")", maxPlistDepth+1))
	manyXML := []byte(`<plist><array>` + strings.Repeat(`<string>x</string>`, maxPlistNodes) + `</array></plist>`)
	objects := []byte{}
	offsets := []uint64{}
	for i := 0; i < maxPlistDepth+1; i++ {
		offsets = append(offsets, uint64(len(objects)+8))
		objects = append(objects, 0xa1, byte((i+1)>>8), byte(i+1))
	}
	offsets = append(offsets, uint64(len(objects)+8))
	objects = append(objects, 0x09)
	deepBinary := plistWithObjects(objects, offsets, 2)
	objects = []byte{0xaf, 0x11, 0x07, 0xd0}
	objects = append(objects, bytes.Repeat([]byte{1}, 2000)...)
	offsets = []uint64{8, uint64(len(objects) + 8)}
	objects = append(objects, 0xaf, 0x11, 0x07, 0xd0)
	objects = append(objects, bytes.Repeat([]byte{2}, 2000)...)
	offsets = append(offsets, uint64(len(objects)+8))
	objects = append(objects, 0x51, 'x')
	expandingBinary := plistWithObjects(objects, offsets, 1)
	utf16Deep := encodeUTF16XML(`<?xml version="1.0" encoding="UTF-16"?>` + string(deepXML))
	malformedXML := []byte(`<?xml version="1.0"?><plist>` + strings.Repeat(`<array>`, maxPlistDepth+1))
	doctypeXML := []byte(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + string(deepXML))
	for _, body := range [][]byte{deepXML, deepText, manyXML, deepBinary, expandingBinary, utf16Deep, malformedXML, doctypeXML} {
		if boundedPlist(body) {
			t.Fatal("oversized structure accepted")
		}
		if _, ok := updatedManifest(body); ok {
			t.Fatal("oversized structure rewritten")
		}
	}
}

func TestBinaryPlistAliasedScalarBudget(t *testing.T) {
	objects := []byte{0xaf, 0x10, 65}
	for index := 1; index <= 65; index++ {
		objects = append(objects, byte(index))
	}
	aliasedOffset := uint64(8 + len(objects))
	offsets := []uint64{8}
	for index := 0; index < 65; index++ {
		offsets = append(offsets, aliasedOffset)
	}
	objects = append(objects, 0x6f, 0x12, 0, 2, 0, 0)
	objects = append(objects, bytes.Repeat([]byte{0, 'a'}, 131072)...)
	body := plistWithObjects(objects, offsets, 1)
	if boundedPlist(body) {
		t.Fatal("aliased UTF16 scalars bypass aggregate decoded byte budget")
	}
	if _, ok := updatedManifest(body); ok {
		t.Fatal("aliased scalars reached manifest rewriting")
	}
	if boundedPlist([]byte(strings.Repeat("x", maxPlistScalarBytes+1))) {
		t.Fatal("text scalar budget not enforced")
	}
}

func FuzzManifestPlistBounds(f *testing.F) {
	for _, format := range []int{plist.XMLFormat, plist.BinaryFormat} {
		body, _ := plist.Marshal(manifestValue(), format)
		f.Add(body)
	}
	f.Add([]byte(`{items=({assets=({kind=software;url="https://cdn.test/a";});});}`))
	f.Add(plistWithObjects([]byte{0x6f, 0x13, 0x80, 0, 0, 0, 0, 0, 0, 0}, []uint64{8}, 1))
	f.Add(encodeUTF16XML(`<?xml version="1.0" encoding="UTF-16"?><plist><dict/></plist>`))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 4*1024*1024 {
			return
		}
		_, _ = updatedManifest(body)
	})
}
