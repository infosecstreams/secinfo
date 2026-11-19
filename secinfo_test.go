package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/infosecstreams/secinfo/streamers"
)

const (
	indexTemplate    = "Header\n---: | --- | :--- | :---\nFooter\n"
	inactiveTemplate = "Header\n--: | ---\nFooter\n"
)

func TestMainWithTestEnvOrdersOutput(t *testing.T) {
	withTempDir(t, func(dir string) {
		writeTemplates(t, dir)
		writeFile(t, filepath.Join(dir, "index.md"), "seed content\n")

		active := streamers.StreamerList{
			Streamers: []streamers.Streamer{
				{Name: "Alpha", ThirtyDayStats: 2},
				{Name: "bravo", ThirtyDayStats: 10},
				{Name: "Charlie", ThirtyDayStats: 5},
			},
		}
		inactive := streamers.StreamerList{
			Streamers: []streamers.Streamer{
				{Name: "Zulu", ThirtyDayStats: 0},
				{Name: "alpha", ThirtyDayStats: 0},
				{Name: "Echo", ThirtyDayStats: 0},
			},
		}
		writeJSON(t, filepath.Join(dir, "active.json"), active)
		writeJSON(t, filepath.Join(dir, "inactive.json"), inactive)

		t.Setenv("SECINFO_TEST", "1")

		main()

		indexOut := readFile(t, filepath.Join(dir, "index.md"))
		inactiveOut := readFile(t, filepath.Join(dir, "inactive.md"))

		assertOrder(t, indexOut, []string{"`bravo`", "`Charlie`", "`Alpha`"})
		assertOrder(t, inactiveOut, []string{"`alpha`", "`Echo`", "`Zulu`"})
	})
}

func TestMainWithoutTestEnvEmptyCsv(t *testing.T) {
	withTempDir(t, func(dir string) {
		writeTemplates(t, dir)
		writeFile(t, filepath.Join(dir, "index.md"), "existing\n")
		writeFile(t, filepath.Join(dir, "streamers.csv"), "")
		writeFile(t, filepath.Join(dir, "inactive_streamers.csv"), "")

		t.Setenv("SECINFO_TEST", "")

		main()

		indexOut := readFile(t, filepath.Join(dir, "index.md"))
		inactiveOut := readFile(t, filepath.Join(dir, "inactive.md"))

		if indexOut != indexTemplate {
			t.Fatalf("index.md should match template when empty: %q", indexOut)
		}
		if inactiveOut != inactiveTemplate {
			t.Fatalf("inactive.md should match template when empty: %q", inactiveOut)
		}
	})
}

func withTempDir(t *testing.T, fn func(dir string)) {
	t.Helper()

	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	fn(dir)
}

func writeTemplates(t *testing.T, dir string) {
	t.Helper()

	templatesDir := filepath.Join(dir, "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("mkdir templates failed: %v", err)
	}
	writeFile(t, filepath.Join(templatesDir, "index.tmpl.md"), indexTemplate)
	writeFile(t, filepath.Join(templatesDir, "inactive.tmpl.md"), inactiveTemplate)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	return string(data)
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write json failed: %v", err)
	}
}

func assertOrder(t *testing.T, content string, items []string) {
	t.Helper()

	last := -1
	for _, item := range items {
		idx := strings.Index(content, item)
		if idx == -1 {
			t.Fatalf("missing %s in output", item)
		}
		if idx <= last {
			t.Fatalf("expected %s after previous item", item)
		}
		last = idx
	}
}

// Garr_7 on 2026-11-05: StreamStatus moved them back to streamers.csv and
// marked them 🟢 when they went live, then the regen demoted them because
// SullyGnome had no 30-day stats for them yet.
func TestSplitActiveKeepsLiveStreamers(t *testing.T) {
	index := strings.Join([]string{
		"🟢 | `Garr_7` | [x](https://www.twitch.tv/Garr_7) &nbsp; | EN",
		"🟢 | `newbie` | [x](https://www.twitch.tv/newbie) &nbsp; | DE",
		"&nbsp; | `idle` | [x](https://www.twitch.tv/idle) &nbsp; |",
		"&nbsp; | `regular` | [x](https://www.twitch.tv/regular) &nbsp; |",
	}, "\n")
	stats := map[string]float32{"regular": 12}
	fetch := func(s *streamers.Streamer) error {
		if s.Name == "newbie" || s.Name == "ghost" {
			return streamers.ErrNotFound // no SullyGnome page yet
		}
		s.SullyGnomeID = "id-" + s.Name
		s.ThirtyDayStats = stats[s.Name]
		return nil
	}
	candidates := []streamers.Streamer{{Name: "Garr_7"}, {Name: "newbie"}, {Name: "idle"}, {Name: "regular"}, {Name: "ghost"}}

	active, inactive, failed := splitActive(candidates, index, fetch)
	if len(failed) != 0 {
		t.Errorf("failed = %v, want none", failed)
	}

	if got := names(active); strings.Join(got, ",") != "Garr_7,newbie,regular" {
		t.Errorf("active = %v, want [Garr_7 newbie regular]", got)
	}
	if got := names(inactive); strings.Join(got, ",") != "idle,ghost" {
		t.Errorf("inactive = %v, want [idle ghost]", got)
	}
	if active[0].Lang != "EN" || active[0].SullyGnomeID != "id-Garr_7" {
		t.Errorf("Garr_7 lost fetched data: %+v", active[0])
	}
}

// End to end through main(): a live 0-stat streamer is written to index.md
// as an online row.
func TestMainRendersLiveZeroStatStreamerOnline(t *testing.T) {
	withTempDir(t, func(dir string) {
		writeTemplates(t, dir)
		writeFile(t, filepath.Join(dir, "index.md"), "🟢 | `Garr_7` | x | EN\n")
		writeJSON(t, filepath.Join(dir, "active.json"), streamers.StreamerList{
			Streamers: []streamers.Streamer{{Name: "Garr_7"}, {Name: "regular", ThirtyDayStats: 3}},
		})
		writeJSON(t, filepath.Join(dir, "inactive.json"), streamers.StreamerList{})
		t.Setenv("SECINFO_TEST", "1")

		main()

		indexOut := readFile(t, filepath.Join(dir, "index.md"))
		if !strings.Contains(indexOut, "🟢 | `Garr_7` |") {
			t.Errorf("Garr_7 not rendered online:\n%s", indexOut)
		}
	})
}

func names(list []streamers.Streamer) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, s.Name)
	}
	return out
}

// 🟢 and 🟡 rows belong to StreamStatus: the regen must keep them as they
// are (category, tags, pre-live time) instead of rebuilding them.
func TestMainKeepsLiveAndPreliveRowsVerbatim(t *testing.T) {
	withTempDir(t, func(dir string) {
		writeTemplates(t, dir)
		live := "🟢 | `alpha` | [x](https://www.twitch.tv/alpha \"Just Chatting, Tags: infosec\") &nbsp; | EN"
		prelive := "🟡 | `Garr_7` | [x](https://www.twitch.tv/Garr_7 \"Getting ready: Science & Technology (since 2026-11-19T14:05Z)\") &nbsp; | EN"
		writeFile(t, filepath.Join(dir, "index.md"), strings.Join([]string{live, prelive, "&nbsp; | `bravo` | old row |"}, "\n")+"\n")
		writeJSON(t, filepath.Join(dir, "active.json"), streamers.StreamerList{Streamers: []streamers.Streamer{
			{Name: "alpha", ThirtyDayStats: 9}, {Name: "Garr_7", ThirtyDayStats: 1}, {Name: "bravo", ThirtyDayStats: 5},
		}})
		writeJSON(t, filepath.Join(dir, "inactive.json"), streamers.StreamerList{})
		t.Setenv("SECINFO_TEST", "1")

		main()

		out := readFile(t, filepath.Join(dir, "index.md"))
		for _, want := range []string{live + "\n", prelive + "\n", "&nbsp; | `bravo` | [<i class"} {
			if !strings.Contains(out, want) {
				t.Errorf("index.md missing %q:\n%s", want, out)
			}
		}
		// Still in 30-day order; status sorting happens in the browser.
		assertOrder(t, out, []string{"`alpha`", "`bravo`", "`Garr_7`"})
	})
}

func TestSplitActiveKeepsPreliveStreamers(t *testing.T) {
	index := "🟡 | `soon` | [x](https://www.twitch.tv/soon \"Getting ready: Just Chatting (since 2026-11-19T14:05Z)\") &nbsp; | FR"
	fetch := func(s *streamers.Streamer) error { s.SullyGnomeID = "x"; return nil }
	active, inactive, _ := splitActive([]streamers.Streamer{{Name: "soon"}}, index, fetch)
	if len(active) != 1 || len(inactive) != 0 || active[0].Lang != "FR" {
		t.Errorf("active=%+v inactive=%+v, want soon kept active with lang FR", active, inactive)
	}
}

// A failed lookup (outage, rate limit, page change) must not demote anyone.
func TestSplitActiveKeepsStreamersWhenLookupFails(t *testing.T) {
	fetch := func(s *streamers.Streamer) error {
		if s.Name == "flaky" {
			return errors.New("HTTP 503")
		}
		return streamers.ErrNotFound
	}
	active, inactive, failed := splitActive([]streamers.Streamer{{Name: "flaky"}, {Name: "gone"}}, "", fetch)
	if strings.Join(names(active), ",") != "flaky" || strings.Join(names(inactive), ",") != "gone" || strings.Join(failed, ",") != "flaky" {
		t.Errorf("active=%v inactive=%v failed=%v; want flaky kept active and reported, gone demoted", names(active), names(inactive), failed)
	}
}

func TestTooManyFailures(t *testing.T) {
	for _, tt := range []struct {
		failed, total int
		want          bool
	}{{0, 42, false}, {2, 10, false}, {3, 42, false}, {5, 42, true}, {42, 42, true}} {
		if got := tooManyFailures(tt.failed, tt.total); got != tt.want {
			t.Errorf("tooManyFailures(%d, %d) = %v, want %v", tt.failed, tt.total, got, tt.want)
		}
	}
}

// A malformed CSV line used to empty streamers.csv and exit 0.
func TestRunRefusesMalformedCSV(t *testing.T) {
	withTempDir(t, func(dir string) {
		writeTemplates(t, dir)
		writeFile(t, filepath.Join(dir, "index.md"), "x\n")
		writeFile(t, filepath.Join(dir, "streamers.csv"), "alice,\nbob\ncarol,\n")
		writeFile(t, filepath.Join(dir, "inactive_streamers.csv"), "dave,\n")
		t.Setenv("SECINFO_TEST", "")

		err := run()
		if err == nil || !strings.Contains(err.Error(), "bob") {
			t.Fatalf("run() = %v, want an error naming the bad line", err)
		}
		if got := readFile(t, filepath.Join(dir, "streamers.csv")); got != "alice,\nbob\ncarol,\n" {
			t.Errorf("streamers.csv was rewritten: %q", got)
		}
	})
}

func TestRunRefusesTemplateWithoutHeading(t *testing.T) {
	withTempDir(t, func(dir string) {
		writeTemplates(t, dir)
		writeFile(t, filepath.Join(dir, "templates", "index.tmpl.md"), "Header\nno table here\n")
		writeFile(t, filepath.Join(dir, "index.md"), "keep me\n")
		t.Setenv("SECINFO_TEST", "1")
		writeJSON(t, filepath.Join(dir, "active.json"), streamers.StreamerList{})
		writeJSON(t, filepath.Join(dir, "inactive.json"), streamers.StreamerList{})

		if err := run(); err == nil {
			t.Fatal("want an error for a template without the table heading")
		}
		if got := readFile(t, filepath.Join(dir, "index.md")); got != "keep me\n" {
			t.Errorf("index.md was rewritten: %q", got)
		}
	})
}

func TestDedupe(t *testing.T) {
	a, i := dedupe(
		[]streamers.Streamer{{Name: "CybersecurityGrowth"}, {Name: "alice"}, {Name: "cybersecuritygrowth"}},
		[]streamers.Streamer{{Name: "Alice"}, {Name: "bob"}},
	)
	if strings.Join(names(a), ",") != "CybersecurityGrowth,alice" || strings.Join(names(i), ",") != "bob" {
		t.Errorf("active=%v inactive=%v", names(a), names(i))
	}
}
