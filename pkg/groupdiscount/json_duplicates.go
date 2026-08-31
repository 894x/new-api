package groupdiscount

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

// rejectDuplicateJSONKeys walks the raw token stream before normal decoding.
// Map decoding alone cannot detect duplicate keys because the last value wins.
// Key strings are decoded through the project's common JSON wrapper so escaped
// equivalents such as "vip" and "v\u0069p" are treated as the same key.
func rejectDuplicateJSONKeys(data []byte) error {
	scanner := duplicateJSONKeyScanner{data: data}
	if err := scanner.scanValue(); err != nil {
		return err
	}
	scanner.skipWhitespace()
	if scanner.offset != len(scanner.data) {
		return errors.New("unexpected data after JSON value")
	}
	return nil
}

type duplicateJSONKeyScanner struct {
	data   []byte
	offset int
}

func (s *duplicateJSONKeyScanner) scanValue() error {
	s.skipWhitespace()
	if s.offset >= len(s.data) {
		return errors.New("unexpected end of JSON input")
	}
	switch s.data[s.offset] {
	case '{':
		return s.scanObject()
	case '[':
		return s.scanArray()
	case '"':
		_, err := s.scanString()
		return err
	default:
		start := s.offset
		for s.offset < len(s.data) {
			switch s.data[s.offset] {
			case ',', '}', ']', ' ', '\t', '\r', '\n':
				if s.offset == start {
					return fmt.Errorf("invalid JSON value at byte %d", start)
				}
				return nil
			default:
				s.offset++
			}
		}
		if s.offset == start {
			return fmt.Errorf("invalid JSON value at byte %d", start)
		}
		return nil
	}
}

func (s *duplicateJSONKeyScanner) scanObject() error {
	s.offset++
	s.skipWhitespace()
	if s.consume('}') {
		return nil
	}
	seen := map[string]struct{}{}
	for {
		s.skipWhitespace()
		if s.offset >= len(s.data) || s.data[s.offset] != '"' {
			return fmt.Errorf("object key must be a JSON string at byte %d", s.offset)
		}
		rawKey, err := s.scanString()
		if err != nil {
			return err
		}
		var key string
		if err := common.Unmarshal(rawKey, &key); err != nil {
			return fmt.Errorf("invalid object key: %w", err)
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate JSON object key %q", key)
		}
		seen[key] = struct{}{}

		s.skipWhitespace()
		if !s.consume(':') {
			return fmt.Errorf("missing colon after object key %q", key)
		}
		if err := s.scanValue(); err != nil {
			return err
		}
		s.skipWhitespace()
		if s.consume('}') {
			return nil
		}
		if !s.consume(',') {
			return fmt.Errorf("missing comma after object value at byte %d", s.offset)
		}
	}
}

func (s *duplicateJSONKeyScanner) scanArray() error {
	s.offset++
	s.skipWhitespace()
	if s.consume(']') {
		return nil
	}
	for {
		if err := s.scanValue(); err != nil {
			return err
		}
		s.skipWhitespace()
		if s.consume(']') {
			return nil
		}
		if !s.consume(',') {
			return fmt.Errorf("missing comma after array value at byte %d", s.offset)
		}
	}
}

func (s *duplicateJSONKeyScanner) scanString() ([]byte, error) {
	start := s.offset
	if !s.consume('"') {
		return nil, fmt.Errorf("JSON string expected at byte %d", s.offset)
	}
	for s.offset < len(s.data) {
		switch s.data[s.offset] {
		case '"':
			s.offset++
			return s.data[start:s.offset], nil
		case '\\':
			s.offset += 2
		default:
			s.offset++
		}
	}
	return nil, errors.New("unterminated JSON string")
}

func (s *duplicateJSONKeyScanner) skipWhitespace() {
	for s.offset < len(s.data) {
		switch s.data[s.offset] {
		case ' ', '\t', '\r', '\n':
			s.offset++
		default:
			return
		}
	}
}

func (s *duplicateJSONKeyScanner) consume(expected byte) bool {
	if s.offset >= len(s.data) || s.data[s.offset] != expected {
		return false
	}
	s.offset++
	return true
}
