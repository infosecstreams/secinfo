package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// botStats builds a stats.json lookup: login -> infosec hours in 30 days.
func botStats(hours map[string]float32) map[string]streamers.BotStreamer {
	out := map[string]streamers.BotStreamer{}
	for login, h := range hours {
		out[strings.ToLower(login)] = streamers.BotStreamer{ID: "id-" + login, Login: strings.ToLower(login), DisplayName: login, Hours30d: h}
	}
	return out
}

// Garr_7 on 2026-11-05: StreamStatus moved them back to streamers.csv and
// marked them 🟢 when they went live, then the regen demoted them because
// they had no 30-day stats yet.
func TestSplitActiveKeepsLiveStreamers(t *testing.T) {
	index := strings.Join([]string{
		"🟢 | `Garr_7` | [x](https://www.twitch.tv/Garr_7) &nbsp; | EN",
		"🟢 | `newbie` | [x](https://www.twitch.tv/newbie) &nbsp; | DE",
		"&nbsp; | `idle` | [x](https://www.twitch.tv/idle) &nbsp; |",
		"&nbsp; | `regular` | [x](https://www.twitch.tv/regular) &nbsp; |",
	}, "\n")
	stats := botStats(map[string]float32{"Garr_7": 0, "newbie": 0, "idle": 0, "regular": 12})
	candidates := []streamers.Streamer{{Name: "Garr_7"}, {Name: "newbie"}, {Name: "idle"}, {Name: "regular"}}

	active, inactive, unknown := splitActive(candidates, index, stats)
	if len(unknown) != 0 {
		t.Errorf("unknown = %v, want none", unknown)
	}
	if got := names(active); strings.Join(got, ",") != "Garr_7,newbie,regular" {
		t.Errorf("active = %v, want [Garr_7 newbie regular]", got)
	}
	if got := names(inactive); strings.Join(got, ",") != "idle" {
		t.Errorf("inactive = %v, want [idle]", got)
	}
	if active[0].Lang != "EN" {
		t.Errorf("Garr_7 lost its language: %+v", active[0])
	}
}

// censoredHacker on 2026-11-12/13: StreamStatus moved them back when they
// went live, and every regen demoted them again minutes later because
// SullyGnome hadn't counted the stream. With StreamStatus's own hours they
// come back and stay, even if StreamStatus missed the live event.
func TestSplitActiveBringsBackInactiveStreamersWithRecentHours(t *testing.T) {
	stats := botStats(map[string]float32{"censoredHacker": 10.4, "dal3ksec": 17, "miloslavpalik": 0.3, "longgone": 0})
	candidates := []streamers.Streamer{
		{Name: "censoredhacker", WasInactive: true},
		{Name: "dal3ksec", WasInactive: true},
		{Name: "miloslavpalik", WasInactive: true},
		{Name: "longgone", WasInactive: true},
	}
	active, inactive, unknown := splitActive(candidates, "", stats)
	if got := strings.Join(names(active), ","); got != "censoredHacker,dal3ksec,miloslavpalik" {
		t.Errorf("active = %s, want censoredHacker (display-name capitalisation), dal3ksec, miloslavpalik", got)
	}
	if got := strings.Join(names(inactive), ","); got != "longgone" || len(unknown) != 0 {
		t.Errorf("inactive = %s unknown = %v, want longgone and none", got, unknown)
	}
	if active[0].ThirtyDayStats != 10.4 {
		t.Errorf("hours = %v, want 10.4", active[0].ThirtyDayStats)
	}
}

// HeckinSpectre on 2026-11-21: one stream, 6% infosec, so ~0 infosec hours.
// They used to stay on the main list showing "0 h" because SullyGnome
// counts every category.
func TestSplitActiveDemotesBelowThreshold(t *testing.T) {
	stats := botStats(map[string]float32{"HeckinSpectre": 0.04, "edge": streamers.ActiveMinHours})
	active, inactive, _ := splitActive([]streamers.Streamer{{Name: "HeckinSpectre"}, {Name: "edge"}}, "", stats)
	if strings.Join(names(active), ",") != "edge" || strings.Join(names(inactive), ",") != "HeckinSpectre" {
		t.Errorf("active=%v inactive=%v; want edge (exactly %.1f h) active, HeckinSpectre inactive", names(active), names(inactive), streamers.ActiveMinHours)
	}
}

func TestSplitActiveKeepsPreliveStreamers(t *testing.T) {
	index := "🟡 | `soon` | [x](https://www.twitch.tv/soon \"Getting ready: Just Chatting (since 2026-11-19T14:05Z)\") &nbsp; | FR"
	active, inactive, _ := splitActive([]streamers.Streamer{{Name: "soon"}}, index, botStats(map[string]float32{"soon": 0}))
	if len(active) != 1 || len(inactive) != 0 || active[0].Lang != "FR" {
		t.Errorf("active=%+v inactive=%+v, want soon kept active with lang FR", active, inactive)
	}
}

// A streamer StreamStatus doesn't know (e.g. a rename it hasn't picked up)
// stays on whichever list they were on.
func TestSplitActiveLeavesUnknownStreamersInPlace(t *testing.T) {
	candidates := []streamers.Streamer{{Name: "renamed"}, {Name: "oldname", WasInactive: true}}
	active, inactive, unknown := splitActive(candidates, "", botStats(map[string]float32{"someoneelse": 3}))
	if strings.Join(names(active), ",") != "renamed" || strings.Join(names(inactive), ",") != "oldname" || strings.Join(unknown, ",") != "renamed,oldname" {
		t.Errorf("active=%v inactive=%v unknown=%v", names(active), names(inactive), unknown)
	}
}

// fakeStats serves body as stats.json for run().
func fakeStats(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	oldURL, oldSleep := streamers.StatsURL, streamers.Sleep
	streamers.StatsURL, streamers.Sleep = srv.URL, func(time.Duration) {}
	t.Cleanup(func() { streamers.StatsURL, streamers.Sleep = oldURL, oldSleep })
}

func statsJSON(hours map[string]float32) string {
	type entry struct {
		ID          string  `json:"id"`
		Login       string  `json:"login"`
		DisplayName string  `json:"display_name"`
		Hours30d    float32 `json:"hours_30d"`
	}
	var list []entry
	for login, h := range hours {
		list = append(list, entry{"id-" + login, strings.ToLower(login), login, h})
	}
	b, _ := json.Marshal(map[string]any{"generated_at": time.Now().UTC().Format(time.RFC3339), "streamers": list})
	return string(b)
}

// End to end: both CSVs are rewritten by the one rule.
func TestRunMovesStreamersBothWays(t *testing.T) {
	withTempDir(t, func(dir string) {
		writeTemplates(t, dir)
		writeFile(t, filepath.Join(dir, "index.md"), "x\n")
		writeFile(t, filepath.Join(dir, "streamers.csv"), "regular,\nquiet,\n")
		writeFile(t, filepath.Join(dir, "inactive_streamers.csv"), "back,\ngone,\n")
		fakeStats(t, 200, statsJSON(map[string]float32{"regular": 5, "quiet": 0, "back": 2.4, "gone": 0}))
		t.Setenv("SECINFO_TEST", "")

		if err := run(); err != nil {
			t.Fatal(err)
		}
		if got := readFile(t, filepath.Join(dir, "streamers.csv")); got != "back,\nregular," {
			t.Errorf("streamers.csv = %q, want back and regular", got)
		}
		if got := readFile(t, filepath.Join(dir, "inactive_streamers.csv")); got != "gone,\nquiet," {
			t.Errorf("inactive_streamers.csv = %q, want gone and quiet", got)
		}
		assertOrder(t, readFile(t, filepath.Join(dir, "index.md")), []string{"`regular`", "`back`"})
	})
}

// No usable stats, no changes: an outage must never move anyone.
func TestRunWritesNothingWithoutStats(t *testing.T) {
	for name, tt := range map[string]struct {
		status int
		body   string
	}{
		"outage":       {503, "starting"},
		"empty":        {200, `{"generated_at":"` + time.Now().UTC().Format(time.RFC3339) + `","streamers":[]}`},
		"stale":        {200, `{"generated_at":"2020-01-01T00:00:00Z","streamers":[{"id":"1","login":"regular","hours_30d":5}]}`},
		"most missing": {200, statsJSON(map[string]float32{"someoneelse": 1})},
	} {
		t.Run(name, func(t *testing.T) {
			withTempDir(t, func(dir string) {
				writeTemplates(t, dir)
				writeFile(t, filepath.Join(dir, "index.md"), "keep\n")
				writeFile(t, filepath.Join(dir, "streamers.csv"), "a,\nb,\nc,\nregular,\n")
				writeFile(t, filepath.Join(dir, "inactive_streamers.csv"), "gone,\n")
				fakeStats(t, tt.status, tt.body)
				t.Setenv("SECINFO_TEST", "")

				if err := run(); err == nil {
					t.Fatal("run() succeeded, want an error")
				}
				if got := readFile(t, filepath.Join(dir, "streamers.csv")); got != "a,\nb,\nc,\nregular,\n" {
					t.Errorf("streamers.csv was rewritten: %q", got)
				}
				if got := readFile(t, filepath.Join(dir, "index.md")); got != "keep\n" {
					t.Errorf("index.md was rewritten: %q", got)
				}
			})
		})
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
