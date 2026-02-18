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
