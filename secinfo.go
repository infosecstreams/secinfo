package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/infosecstreams/secinfo/streamers"
)

// Table headings in the templates; rows are inserted right after them.
const (
	indexHeading    = "---: | --- | :--- | :---\n"
	inactiveHeading = "--: | ---\n"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "secinfo: %s\n", err)
		os.Exit(1)
	}
}

// run regenerates the streamer CSVs, JSON files and markdown pages. It
// checks every input before writing anything, and fails rather than
// writing a list that lost streamers because of bad input or an outage.
func run() error {
	test := os.Getenv("SECINFO_TEST") != ""

	indexMd, err := os.ReadFile("index.md")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading index.md: %w", err)
	}
	indexStr := string(indexMd)

	indexTmpl, err := readTemplate("templates/index.tmpl.md", indexHeading)
	if err != nil {
		return err
	}
	inactiveTmpl, err := readTemplate("templates/inactive.tmpl.md", inactiveHeading)
	if err != nil {
		return err
	}

	active := streamers.StreamerList{}
	inactive := streamers.StreamerList{}
	if !test {
		activeFromFile, err := readStreamers("streamers.csv", true)
		if err != nil {
			return err
		}
		inactiveFromFile, err := readStreamers("inactive_streamers.csv", false)
		if err != nil {
			return err
		}
		activeFromFile, inactiveFromFile = dedupe(activeFromFile, inactiveFromFile)
		for i := range inactiveFromFile {
			inactiveFromFile[i].WasInactive = true
		}

		// Only active streamers are checked against SullyGnome; inactive
		// ones stay inactive until StreamStatus sees them go live.
		var failed []string
		active.Streamers, inactive.Streamers, failed = splitActive(activeFromFile, indexStr, fetchSullyGnome)
		if tooManyFailures(len(failed), len(activeFromFile)) {
			return fmt.Errorf("SullyGnome lookups failed for %d of %d streamers (%s); not writing anything",
				len(failed), len(activeFromFile), strings.Join(failed, ", "))
		}
		inactive.Streamers = append(inactive.Streamers, inactiveFromFile...)
	} else {
		if err := readJSON("active.json", &active); err != nil {
			return err
		}
		if err := readJSON("inactive.json", &inactive); err != nil {
			return err
		}
	}

	active.Sort()   // by ThirtyDayStats, descending
	inactive.Sort() // for the JSON file

	// Build every output before writing any of them.
	var b strings.Builder
	b.WriteString(indexTmpl.before)
	for _, streamer := range active.Streamers {
		b.WriteString(activeRow(streamer, indexStr))
	}
	b.WriteString(indexTmpl.after)
	newIndex := b.String()

	inactiveByName := streamers.StreamerList{Streamers: append([]streamers.Streamer(nil), inactive.Streamers...)}
	inactiveByName.SortByName()
	b.Reset()
	b.WriteString(inactiveTmpl.before)
	for _, streamer := range inactiveByName.Streamers {
		line, err := streamer.ReturnMarkdownLine(false) // inactive rows have no status
		if err != nil {
			return err
		}
		b.WriteString(line)
	}
	b.WriteString(inactiveTmpl.after)
	newInactive := b.String()

	if !test {
		activeJSON, err := json.Marshal(active)
		if err != nil {
			return err
		}
		inactiveJSON, err := json.Marshal(inactive)
		if err != nil {
			return err
		}
		for _, w := range []struct {
			path string
			data []byte
		}{{"active.json", activeJSON}, {"inactive.json", inactiveJSON}} {
			if err := os.WriteFile(w.path, w.data, 0644); err != nil {
				return fmt.Errorf("writing %s: %w", w.path, err)
			}
		}
		// CSVs are written sorted by name for human readability.
		if err := active.WriteCSV("streamers.csv"); err != nil {
			return fmt.Errorf("writing streamers.csv: %w", err)
		}
		if err := inactive.WriteCSV("inactive_streamers.csv"); err != nil {
			return fmt.Errorf("writing inactive_streamers.csv: %w", err)
		}
	}
	if err := os.WriteFile("index.md", []byte(newIndex), 0644); err != nil {
		return fmt.Errorf("writing index.md: %w", err)
	}
	if err := os.WriteFile("inactive.md", []byte(newInactive), 0644); err != nil {
		return fmt.Errorf("writing inactive.md: %w", err)
	}
	return nil
}

// template is a page template split at its table heading.
type template struct{ before, after string }

func readTemplate(path, heading string) (template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return template{}, fmt.Errorf("reading %s: %w", path, err)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	i := strings.Index(text, heading)
	if i < 0 {
		return template{}, fmt.Errorf("%s has no table heading %q", path, strings.TrimSpace(heading))
	}
	i += len(heading)
	return template{before: text[:i], after: text[i:]}, nil
}

// readStreamers parses a streamers CSV. A malformed line fails the run
// instead of silently dropping the whole list. A missing file is only an
// error when required; an empty file is an empty list.
func readStreamers(path string, required bool) ([]streamers.Streamer, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && !required {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	list, err := streamers.ParseCSV(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return list.Streamers, nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// dedupe drops repeated names (case-insensitive) within and across the two
// lists; the first active entry wins.
func dedupe(active, inactive []streamers.Streamer) ([]streamers.Streamer, []streamers.Streamer) {
	seen := map[string]bool{}
	keep := func(list []streamers.Streamer) []streamers.Streamer {
		var out []streamers.Streamer
		for _, s := range list {
			key := strings.ToLower(s.Name)
			if seen[key] {
				fmt.Printf("dropping duplicate streamer entry: %s\n", s.Name)
				continue
			}
			seen[key] = true
			out = append(out, s)
		}
		return out
	}
	a := keep(active)
	return a, keep(inactive)
}

// tooManyFailures reports whether so many lookups failed that SullyGnome is
// probably down or has changed; then nothing should be written.
func tooManyFailures(failed, total int) bool {
	return failed > 2 && failed*10 > total
}

// fetchSullyGnome looks the streamer up on SullyGnome and loads their
// 30-day stats. It returns streamers.ErrNotFound if SullyGnome has no page
// for them and another error if the lookup failed.
func fetchSullyGnome(s *streamers.Streamer) error {
	if err := s.GetUID(); err != nil {
		return err
	}
	return s.GetStats()
}

// splitActive decides which streamers from streamers.csv stay active:
//   - stats loaded: active if they streamed in the last 30 days
//   - not on SullyGnome: inactive
//   - lookup failed: kept active (nothing is known), and reported in failed
//
// Anyone 🟢 live or 🟡 about to go live in index.md stays active
// regardless: a streamer StreamStatus just brought back has no recent
// stats yet and must not be demoted mid-stream.
func splitActive(candidates []streamers.Streamer, indexText string, fetch func(*streamers.Streamer) error) (active, inactive []streamers.Streamer, failed []string) {
	for _, streamer := range candidates {
		err := fetch(&streamer)
		switch {
		case err == nil && streamer.ThirtyDayStats > 0:
			active = append(active, streamer)
		case hasLiveStatus(&streamer, indexText):
			fmt.Printf("keeping live/pre-live streamer active despite no 30-day stats: %s\n", streamer.Name)
			active = append(active, streamer)
		case err == nil, errors.Is(err, streamers.ErrNotFound):
			fmt.Printf("moving %s to inactive (no streams in 30 days)\n", streamer.Name)
			inactive = append(inactive, streamer)
		default:
			fmt.Printf("SullyGnome lookup failed for %s, keeping them active: %s\n", streamer.Name, err)
			failed = append(failed, streamer.Name)
			active = append(active, streamer)
		}
	}
	return active, inactive, failed
}

// hasLiveStatus reports whether the streamer's index.md row is 🟢 or 🟡.
func hasLiveStatus(s *streamers.Streamer, indexText string) bool {
	row, status := s.IndexRow(indexText)
	if status == "" {
		return false
	}
	// Rows end in "| <LANG>"; keep it for active.json as before.
	if len(row) >= 2 {
		s.Lang = row[len(row)-2:]
	}
	return true
}

// activeRow returns the streamer's index.md row. 🟢 and 🟡 rows are owned
// by StreamStatus (they carry the stream's category, tags and pre-live
// time), so they are kept verbatim; everything else is regenerated.
func activeRow(s streamers.Streamer, indexText string) string {
	if row, status := s.IndexRow(indexText); status != "" {
		return row + "\n"
	}
	line, err := s.ReturnMarkdownLine(false)
	if err != nil {
		fmt.Println(err)
	}
	return line
}
