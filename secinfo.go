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

		// One rule for both lists, from StreamStatus's own data: see
		// streamers.ActiveMinHours. No stats, no changes.
		stats, err := streamers.LoadBotStats()
		if err != nil {
			return fmt.Errorf("loading StreamStatus stats: %w", err)
		}
		candidates := append(activeFromFile, inactiveFromFile...)
		var unknown []string
		active.Streamers, inactive.Streamers, unknown = splitActive(candidates, indexStr, stats)
		if tooManyFailures(len(unknown), len(candidates)) {
			return fmt.Errorf("StreamStatus has no stats for %d of %d streamers (%s); not writing anything",
				len(unknown), len(candidates), strings.Join(unknown, ", "))
		}
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

// tooManyFailures reports whether so many streamers are missing from
// StreamStatus's stats that its data is probably wrong; then nothing should
// be written.
func tooManyFailures(failed, total int) bool {
	return failed > 2 && failed*10 > total
}

// splitActive applies the roster rule to every streamer from both CSVs:
//   - 🟢 live or 🟡 about to go live in index.md: active
//   - StreamStatus has at least streamers.ActiveMinHours infosec hours for
//     them in the last 30 days: active (inactive ones come back)
//   - StreamStatus knows them with less: inactive
//   - StreamStatus doesn't know them (e.g. a rename it hasn't seen yet):
//     left on the list they were on, and reported in unknown
func splitActive(candidates []streamers.Streamer, indexText string, stats map[string]streamers.BotStreamer) (active, inactive []streamers.Streamer, unknown []string) {
	for _, streamer := range candidates {
		known := streamer.ApplyBotStats(stats)
		switch {
		case known && streamer.Active():
			if streamer.WasInactive {
				fmt.Printf("moving %s back to active (%.1f infosec hours in 30 days)\n", streamer.Name, streamer.ThirtyDayStats)
			}
			active = append(active, streamer)
		case hasLiveStatus(&streamer, indexText):
			active = append(active, streamer)
		case known:
			if !streamer.WasInactive {
				fmt.Printf("moving %s to inactive (%.1f infosec hours in 30 days)\n", streamer.Name, streamer.ThirtyDayStats)
			}
			inactive = append(inactive, streamer)
		default:
			fmt.Printf("StreamStatus has no stats for %s; leaving them where they are\n", streamer.Name)
			unknown = append(unknown, streamer.Name)
			if streamer.WasInactive {
				inactive = append(inactive, streamer)
			} else {
				active = append(active, streamer)
			}
		}
	}
	return active, inactive, unknown
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
