package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

const cmpDir = "/tmp/claude-0/-home-user-rpiv-mono/69046b9c-d7f1-58e6-848f-3c05629a3109/scratchpad/widthcmp"

func TestZZSweepAgainstPiG(t *testing.T) {
	t.Logf("Go unicode.Version = %s", unicode.Version)
	f, err := os.Open(cmpDir + "/sweep.txt")
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	type mm struct {
		r          rune
		col        int
		want, got  int
	}
	var mismatches []mm
	counts := map[string]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		rv, _ := strconv.ParseInt(fields[0], 16, 32)
		r := rune(rv)
		s := string(r)
		inputs := []string{s, s + "́", s + "️", "a" + s + "b"}
		for col, in := range inputs {
			want, _ := strconv.Atoi(fields[col+1])
			got := VisibleWidth(in)
			if got != want {
				key := fmt.Sprintf("col%d", col)
				counts[key]++
				if counts[key] <= 60 {
					mismatches = append(mismatches, mm{r, col, want, got})
				}
			}
		}
	}
	t.Logf("mismatch counts per column (0=lone, 1=+U+0301, 2=+FE0F, 3=a_b): %v", counts)
	// Group lone mismatches into ranges for readability.
	var lone []rune
	for _, m := range mismatches {
		if m.col == 0 {
			lone = append(lone, m.r)
		}
	}
	for _, m := range mismatches {
		if m.col != 0 || len(lone) < 60 {
			assigned := unicode.Is(unicode.Cn, m.r)
			t.Logf("col%d U+%04X %q: PiG=%d got=%d (go-unassigned=%v cat=%s)", m.col, m.r, string(m.r), m.want, m.got, assigned, catName(m.r))
		}
	}
	if len(counts) > 0 {
		t.Errorf("sweep mismatches: %v", counts)
	}
}

func catName(r rune) string {
	for name, tab := range unicode.Categories {
		if len(name) == 2 && unicode.Is(tab, r) {
			return name
		}
	}
	return "Cn"
}

// Full-range dump of lone mismatches to a file for analysis.
func TestZZSweepDumpRanges(t *testing.T) {
	f, err := os.Open(cmpDir + "/sweep.txt")
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	out, _ := os.Create(cmpDir + "/mismatch_ranges.txt")
	defer out.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var startR, prevR rune = -1, -1
	var curKey string
	flush := func() {
		if startR >= 0 {
			fmt.Fprintf(out, "%s U+%04X..U+%04X (%d) %s\n", curKey, startR, prevR, prevR-startR+1, catName(startR))
		}
	}
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		rv, _ := strconv.ParseInt(fields[0], 16, 32)
		r := rune(rv)
		s := string(r)
		inputs := []string{s, s + "́", s + "️", "a" + s + "b"}
		var keys []string
		for col, in := range inputs {
			want, _ := strconv.Atoi(fields[col+1])
			got := VisibleWidth(in)
			if got != want {
				keys = append(keys, fmt.Sprintf("c%d:%d->%d", col, want, got))
			}
		}
		key := strings.Join(keys, ",")
		if key == "" {
			flush()
			startR = -1
			continue
		}
		if startR >= 0 && key == curKey && r == prevR+1 {
			prevR = r
			continue
		}
		flush()
		startR, prevR, curKey = r, r, key
	}
	flush()
}

func TestZZCorpusAgainstPiG(t *testing.T) {
	data, err := os.ReadFile(cmpDir + "/corpus.json")
	if err != nil {
		t.Skip(err)
	}
	type truncCase struct {
		Width    int    `json:"w"`
		Ellipsis string `json:"e"`
		Pad      bool   `json:"p"`
		Want     string `json:"want"`
	}
	type wrapCase struct {
		Width int      `json:"w"`
		Want  []string `json:"want"`
	}
	type entry struct {
		Input string      `json:"in"`
		Width int         `json:"vw"`
		Trunc []truncCase `json:"trunc"`
		Wrap  []wrapCase  `json:"wrap"`
	}
	var entries []entry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	vw, tr, wr := 0, 0, 0
	shown := map[string]int{}
	for _, e := range entries {
		if got := VisibleWidth(e.Input); got != e.Width {
			vw++
			if shown["vw"] < 25 {
				shown["vw"]++
				t.Logf("VisibleWidth(%q) = %d, PiG %d", e.Input, got, e.Width)
			}
		}
		for _, tc := range e.Trunc {
			if got := TruncateToWidth(e.Input, tc.Width, tc.Ellipsis, tc.Pad); got != tc.Want {
				tr++
				if shown["tr"] < 25 {
					shown["tr"]++
					t.Logf("TruncateToWidth(%q, %d, %q, %v)\n  got  %q\n  PiG  %q", e.Input, tc.Width, tc.Ellipsis, tc.Pad, got, tc.Want)
				}
			}
		}
		for _, wc := range e.Wrap {
			if got := WrapTextWithAnsi(e.Input, wc.Width); !slices.Equal(got, wc.Want) {
				wr++
				if shown["wr"] < 25 {
					shown["wr"]++
					t.Logf("WrapTextWithAnsi(%q, %d)\n  got  %q\n  PiG  %q", e.Input, wc.Width, got, wc.Want)
				}
			}
		}
	}
	t.Logf("entries=%d visibleWidth mismatches=%d truncate mismatches=%d wrap mismatches=%d", len(entries), vw, tr, wr)
	if vw+tr+wr > 0 {
		t.Errorf("corpus mismatches: vw=%d tr=%d wr=%d", vw, tr, wr)
	}
}
