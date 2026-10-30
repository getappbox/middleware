package middleware

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

const requestBodyLimit = 16 * 1024

func decodeBody(w http.ResponseWriter, r *http.Request, target any, required []string, message string) error {
	body, err := readRequestBody(w, r)
	invalid := apiError{400, "invalid_body", message}
	if err != nil {
		var legacy legacyError
		if errors.As(err, &legacy) {
			return legacy
		}
		return invalid
	}
	if !utf8.Valid(body) {
		return invalid
	}
	mediaType, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	var fields map[string]json.RawMessage
	switch mediaType {
	case "application/json", "application/vnd.api+json":
		fields, err = firstJSONMembers(body)
		if err != nil {
			return invalid
		}
	case "application/x-www-form-urlencoded", "multipart/form-data":
		values := make(map[string][]string)
		if mediaType == "application/x-www-form-urlencoded" {
			for pair := range strings.SplitSeq(string(body), "&") {
				name, raw, ok := strings.Cut(pair, "=")
				if !ok {
					continue
				}
				name, err = url.QueryUnescape(name)
				if err != nil || !utf8.ValidString(name) {
					return invalid
				}
				values[name] = append(values[name], raw)
			}
		} else {
			if params["boundary"] == "" {
				return invalid
			}
			reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
			for {
				part, err := reader.NextPart()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return invalid
				}
				data, err := io.ReadAll(part)
				if err != nil || !utf8.Valid(data) || !utf8.ValidString(part.FormName()) {
					return invalid
				}
				values[part.FormName()] = append(values[part.FormName()], url.QueryEscape(string(data)))
			}
		}
		scalars := append([]string{}, required...)
		switch target.(type) {
		case *shortenRequest:
			scalars = append(scalars, "provider")
		case *mailRequest:
			scalars = append(scalars, "personalMessage")
		}
		fields = make(map[string]json.RawMessage)
		for name, entries := range values {
			for _, scalar := range scalars {
				if scalar != "to" && strings.HasPrefix(name, scalar+"[") {
					return invalid
				}
			}
			var value string
			for _, raw := range entries {
				value, err = url.QueryUnescape(raw)
				if err != nil || !utf8.ValidString(value) {
					return invalid
				}
			}
			fields[name], _ = json.Marshal(value)
		}
		if _, mail := target.(*mailRequest); mail {
			recipients, err := formRecipients(values)
			if err != nil {
				return invalid
			}
			fields["to"], _ = json.Marshal(recipients)
		}
	default:
		return invalid
	}
	for _, name := range required {
		if field, ok := fields[name]; !ok || bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
			return invalid
		}
	}
	if raw, exists := fields["to"]; exists && isMailRequest(target) {
		var recipients []*string
		if json.Unmarshal(raw, &recipients) != nil {
			return invalid
		}
		for _, address := range recipients {
			if address == nil {
				return invalid
			}
		}
	}
	allowed := append([]string{}, required...)
	switch target.(type) {
	case *shortenRequest:
		allowed = append(allowed, "provider")
	case *mailRequest:
		allowed = append(allowed, "personalMessage")
	}
	known := make(map[string]json.RawMessage)
	for _, name := range allowed {
		if value, exists := fields[name]; exists {
			known[name] = value
		}
	}
	body, _ = json.Marshal(known)
	if json.Unmarshal(body, target) != nil {
		return invalid
	}
	return nil
}

func readRequestBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	encoded, err := io.ReadAll(http.MaxBytesReader(w, r.Body, requestBodyLimit))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return nil, legacyError{413, "Payload Too Large"}
	}
	if err != nil {
		return nil, err
	}
	encoding, _, _ := strings.Cut(r.Header.Get("Content-Encoding"), ",")
	encoding = strings.ToLower(strings.TrimSpace(encoding))
	if encoding != "gzip" && encoding != "deflate" {
		return encoded, nil
	}
	input := bytes.NewReader(encoded)
	var decoder io.ReadCloser
	if len(encoded) >= 2 && encoded[0] == 0x1f && encoded[1] == 0x8b {
		gzipReader, err := gzip.NewReader(input)
		if err != nil {
			return nil, err
		}
		gzipReader.Multistream(false)
		decoder = gzipReader
	} else {
		decoder, err = zlib.NewReader(input)
		if err != nil {
			return nil, err
		}
	}
	defer decoder.Close()
	limit := min(requestBodyLimit, len(encoded)*25)
	body, err := io.ReadAll(io.LimitReader(decoder, int64(limit+1)))
	if len(body) > limit {
		return nil, legacyError{413, "Payload Too Large"}
	}
	if err != nil {
		return nil, err
	}
	if input.Len() != 0 {
		return nil, errors.New("invalid trailing compressed request data")
	}
	return body, nil
}

func firstJSONMembers(body []byte) (map[string]json.RawMessage, error) {
	if !utf8.Valid(body) || !json.Valid(body) || !validJSONSurrogates(body) {
		return nil, errors.New("invalid JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return nil, errors.New("expected JSON object")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok {
			return nil, errors.New("invalid JSON member")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if _, exists := fields[name]; !exists {
			fields[name] = value
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return fields, nil
}

func validJSONSurrogates(body []byte) bool {
	inString := false
	for index := 0; index < len(body); index++ {
		switch body[index] {
		case '"':
			inString = !inString
		case '\\':
			if !inString {
				continue
			}
			index++
			if index >= len(body) {
				return false
			}
			if body[index] != 'u' {
				continue
			}
			if index+4 >= len(body) {
				return false
			}
			value, err := strconv.ParseUint(string(body[index+1:index+5]), 16, 16)
			if err != nil {
				return false
			}
			index += 4
			if value >= 0xdc00 && value <= 0xdfff {
				return false
			}
			if value >= 0xd800 && value <= 0xdbff {
				if index+6 >= len(body) || body[index+1] != '\\' || body[index+2] != 'u' {
					return false
				}
				next, err := strconv.ParseUint(string(body[index+3:index+7]), 16, 16)
				if err != nil || next < 0xdc00 || next > 0xdfff {
					return false
				}
				index += 6
			}
		}
	}
	return true
}

func isMailRequest(target any) bool { _, ok := target.(*mailRequest); return ok }

func formRecipients(values map[string][]string) ([]string, error) {
	indexed := make(map[int]string)
	for key, entries := range values {
		if !strings.HasPrefix(key, "to[") || key == "to[]" {
			continue
		}
		index, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(key, "to["), "]"))
		if err != nil || index < 0 || !strings.HasSuffix(key, "]") {
			return nil, errors.New("invalid recipient index")
		}
		indexed[index] = entries[len(entries)-1]
	}
	result := []string{}
	if len(indexed) > 0 {
		for index := 0; index < len(indexed); index++ {
			raw, exists := indexed[index]
			if !exists {
				return nil, errors.New("nonsequential recipient indexes")
			}
			value, err := url.QueryUnescape(raw)
			if err != nil || !utf8.ValidString(value) {
				return nil, errors.New("invalid recipient encoding")
			}
			result = append(result, value)
		}
		return result, nil
	}
	for _, raw := range append(append([]string{}, values["to"]...), values["to[]"]...) {
		for _, part := range strings.Split(strings.ReplaceAll(raw, "|", ","), ",") {
			value, err := url.QueryUnescape(part)
			if err != nil || !utf8.ValidString(value) {
				return nil, errors.New("invalid recipient encoding")
			}
			result = append(result, value)
		}
	}
	return result, nil
}
