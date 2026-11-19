package main

import (
	"encoding/json"
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
	fetch := func(s *streamers.Streamer) bool {
		if s.Name == "newbie" || s.Name == "ghost" {
			return false // no SullyGnome ID yet
		}
		s.SullyGnomeID = "id-" + s.Name
		s.ThirtyDayStats = stats[s.Name]
		return true
	}
	candidates := []streamers.Streamer{{Name: "Garr_7"}, {Name: "newbie"}, {Name: "idle"}, {Name: "regular"}, {Name: "ghost"}}

	active, inactive := splitActive(candidates, index, fetch)

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
