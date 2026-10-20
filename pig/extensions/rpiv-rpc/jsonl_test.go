package rpc

import (
	"errors"
	"strings"
	"testing"
)

func readAll(t *testing.T, input string) []string {
	t.Helper()
	var lines []string
	err := ReadLines(strings.NewReader(input), func(line []byte) error {
		lines = append(lines, string(line))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return lines
}

func TestReadLinesFraming(t *testing.T) {
	large := strings.Repeat("x", 3<<20)
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"lf", "{\"a\":1}\n{\"b\":2}\n", []string{`{"a":1}`, `{"b":2}`}},
		{"crlf", "{\"a\":1}\r\n{\"b\":2}\r\n", []string{`{"a":1}`, `{"b":2}`}},
		{"one cr only", "a\r\r\n", []string{"a\r"}},
		{"unterminated tail", "{\"a\":1}\n{\"b\":2}", []string{`{"a":1}`, `{"b":2}`}},
		{"empty record", "\n\n{\"a\":1}\n", []string{"", "", `{"a":1}`}},
		{"embedded line separator", "{\"t\":\"a\u2028b\u2029c\"}\n", []string{"{\"t\":\"a\u2028b\u2029c\"}"}},
		{"embedded cr", "{\"t\":\"a\rb\"}\n", []string{"{\"t\":\"a\rb\"}"}},
		{"large line", "{\"t\":\"" + large + "\"}\n{\"a\":1}\n", []string{"{\"t\":\"" + large + "\"}", `{"a":1}`}},
		{"empty input", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := readAll(t, tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("%d records, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("record %d: %q, want %q", i, truncateForLog(got[i]), truncateForLog(tc.want[i]))
				}
			}
		})
	}
}

func truncateForLog(s string) string {
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}

func TestReadLinesStopsOnCallbackError(t *testing.T) {
	stop := errors.New("stop")
	var seen int
	err := ReadLines(strings.NewReader("a\nb\nc\n"), func([]byte) error {
		seen++
		if seen == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || seen != 2 {
		t.Fatalf("err %v after %d records", err, seen)
	}
}
