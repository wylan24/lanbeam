package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanName(t *testing.T) {
	cases := map[string]string{
		"photo.jpg":        "photo.jpg",
		"../../etc/passwd": "passwd",
		`C:\Users\x\a.txt`: "a.txt",
		"what?.txt":        "what_.txt",
		"  .hidden  ":      "hidden",
		"":                 "file",
		"..":               "file",
		"CON.txt":          "_CON.txt",
		"nul":              "_nul",
		"résumé 📄.pdf":     "résumé 📄.pdf",
		"a\x00b":           "a_b",
	}
	for in, want := range cases {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%q) = %q, want %q", in, got, want)
		}
	}
	long := cleanName(strings.Repeat("é", 300) + ".mov")
	if len(long) > 200 || !strings.HasSuffix(long, ".mov") {
		t.Errorf("long name not truncated correctly: %d bytes", len(long))
	}
}

func newTestServer(t *testing.T) (*server, string) {
	dir := t.TempDir()
	return newServer(dir, "secret-token-123456", []string{"http://192.168.1.2:8765/?t=secret-token-123456"}), dir
}

func do(s *server, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestAuth(t *testing.T) {
	s, _ := newTestServer(t)
	if c := do(s, "GET", "/api/files", "").Code; c != http.StatusUnauthorized {
		t.Fatalf("no token: got %d", c)
	}
	if c := do(s, "GET", "/api/files?t=wrong", "").Code; c != http.StatusUnauthorized {
		t.Fatalf("wrong token: got %d", c)
	}
	if c := do(s, "GET", "/?t=secret-token-123456", "").Code; c != http.StatusOK {
		t.Fatalf("right token: got %d", c)
	}
}

func TestUploadDownloadDelete(t *testing.T) {
	s, dir := newTestServer(t)
	const tok = "?t=secret-token-123456"

	for i := 0; i < 2; i++ {
		rec := do(s, "POST", "/api/upload"+tok+"&name=..%2Fnote.txt", "hello")
		if rec.Code != http.StatusOK {
			t.Fatalf("upload: %d %s", rec.Code, rec.Body)
		}
	}
	for _, name := range []string{"note.txt", "note (1).txt"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(b) != "hello" {
			t.Fatalf("%s: %q %v", name, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "note.txt")); err == nil {
		t.Fatal("upload escaped the folder")
	}

	list := do(s, "GET", "/api/files"+tok, "").Body.String()
	if !strings.Contains(list, `"note (1).txt"`) || strings.Contains(list, ".part") {
		t.Fatalf("unexpected list: %s", list)
	}

	rec := do(s, "GET", "/dl/note%20(1).txt"+tok, "")
	if rec.Code != http.StatusOK || rec.Body.String() != "hello" {
		t.Fatalf("download: %d %q", rec.Code, rec.Body)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("missing attachment header")
	}
	if c := do(s, "GET", "/dl/..%2Fsecret"+tok, "").Code; c != http.StatusNotFound {
		t.Fatalf("traversal download: got %d", c)
	}

	if c := do(s, "DELETE", "/api/files/note.txt"+tok, "").Code; c != http.StatusNoContent {
		t.Fatalf("delete: got %d", c)
	}
	if _, err := os.Stat(filepath.Join(dir, "note.txt")); err == nil {
		t.Fatal("file still exists after delete")
	}
}

func TestOpenFolderOnlyLocal(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/open-folder?t=secret-token-123456", nil)
	req.RemoteAddr = "192.168.1.50:5000"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("remote open-folder: got %d", rec.Code)
	}
}
