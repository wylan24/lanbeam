package main

import (
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

//go:embed web/index.html
var indexHTML []byte

//go:embed web/locked.html
var lockedHTML []byte

type server struct {
	dir   string
	token string
	urls  []string
	hub   *hub
	mu    sync.Mutex // serialises choosing a free file name
	mux   *http.ServeMux
}

func newServer(dir, token string, urls []string) *server {
	s := &server{dir: dir, token: token, urls: urls, hub: newHub()}
	m := http.NewServeMux()
	m.HandleFunc("GET /{$}", s.index)
	m.HandleFunc("GET /api/info", s.info)
	m.HandleFunc("GET /api/qr", s.qr)
	m.HandleFunc("GET /api/files", s.list)
	m.HandleFunc("POST /api/upload", s.upload)
	m.HandleFunc("DELETE /api/files/{name}", s.remove)
	m.HandleFunc("GET /api/events", s.events)
	m.HandleFunc("POST /api/open-folder", s.openFolder)
	m.HandleFunc("GET /dl/{name}", s.download)
	s.mux = m
	return s
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	if !s.authorized(r) {
		if r.URL.Path == "/" {
			h.Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write(lockedHTML)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.URL.Query().Get("t") != "" {
		http.SetCookie(w, &http.Cookie{
			Name: "lanbeam", Value: s.token, Path: "/",
			HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 30 * 24 * 3600,
		})
	}
	s.mux.ServeHTTP(w, r)
}

func (s *server) authorized(r *http.Request) bool {
	t := r.URL.Query().Get("t")
	if t == "" {
		t = r.Header.Get("X-Token")
	}
	if t == "" {
		if c, err := r.Cookie("lanbeam"); err == nil {
			t = c.Value
		}
	}
	return subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) == 1
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(indexHTML)
}

func (s *server) info(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	writeJSON(w, map[string]any{
		"host":    host,
		"version": version,
		"local":   isLoopback(r),
		"urls":    s.urls,
		"dir":     s.dir,
	})
}

func (s *server) qr(w http.ResponseWriter, r *http.Request) {
	i, _ := strconv.Atoi(r.URL.Query().Get("i"))
	if i < 0 || i >= len(s.urls) {
		http.NotFound(w, r)
		return
	}
	svg, err := qrSVG(s.urls[i])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Write(svg)
}

type fileEntry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Mod  int64  `json:"mod"` // unix milliseconds
}

func (s *server) list(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	files := []fileEntry{}
	for _, e := range entries {
		if !validName(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, fileEntry{e.Name(), info.Size(), info.ModTime().UnixMilli()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Mod > files[j].Mod })
	writeJSON(w, files)
}

// upload streams the raw request body to disk, so files of any size work
// without buffering them in memory.
func (s *server) upload(w http.ResponseWriter, r *http.Request) {
	name := cleanName(r.URL.Query().Get("name"))
	tmp, err := os.CreateTemp(s.dir, ".lanbeam-*.part")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	n, copyErr := io.Copy(tmp, r.Body)
	closeErr := tmp.Close()
	if copyErr == nil && r.ContentLength >= 0 && n != r.ContentLength {
		copyErr = io.ErrUnexpectedEOF
	}
	if err := errors.Join(copyErr, closeErr); err != nil {
		os.Remove(tmp.Name())
		http.Error(w, "upload failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	final, err := s.place(tmp.Name(), name)
	if err != nil {
		os.Remove(tmp.Name())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.hub.notify()
	writeJSON(w, fileEntry{Name: final, Size: n, Mod: time.Now().UnixMilli()})
}

// place moves tmp into the folder under name, adding " (1)", " (2)", … if
// that name is taken.
func (s *server) place(tmp, name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 0; i < 10000; i++ {
		cand := name
		if i > 0 {
			cand = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		p := filepath.Join(s.dir, cand)
		if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
			return cand, os.Rename(tmp, p)
		}
	}
	return "", errors.New("no free file name")
}

func (s *server) download(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validName(name) {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(s.dir, name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	disp := "attachment"
	if r.URL.Query().Has("inline") {
		disp = "inline"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": name}))
	// Opened files must never run script with access to this origin.
	w.Header().Set("Content-Security-Policy", "sandbox")
	http.ServeContent(w, r, name, st.ModTime(), f)
}

func (s *server) remove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validName(name) {
		http.NotFound(w, r)
		return
	}
	if err := os.Remove(filepath.Join(s.dir, name)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.hub.notify()
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) openFolder(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		http.Error(w, "only available on the computer running LanBeam", http.StatusForbidden)
		return
	}
	if err := openPath(s.dir); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// events tells open pages when the file list changes (server-sent events).
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	io.WriteString(w, "retry: 2000\n\n")
	fl.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			io.WriteString(w, "data: changed\n\n")
		case <-ping.C:
			io.WriteString(w, ": ping\n\n")
		}
		fl.Flush()
	}
}

type hub struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func newHub() *hub { return &hub{subs: map[chan struct{}]struct{}{}} }

func (h *hub) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan struct{}) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *hub) notify() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// cleanName turns a client-supplied file name into one that is safe on
// Windows, macOS and Linux and cannot escape the folder.
func cleanName(s string) string {
	s = path.Base(strings.ReplaceAll(s, `\`, "/"))
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"/\|?*`, r) {
			b.WriteRune('_')
		} else {
			b.WriteRune(r)
		}
	}
	s = strings.Trim(b.String(), " .")
	if s == "" {
		s = "file"
	}
	if stem, _, _ := strings.Cut(s, "."); reservedNames[strings.ToUpper(stem)] {
		s = "_" + s
	}
	if len(s) > 200 {
		ext := filepath.Ext(s)
		if len(ext) > 20 {
			ext = ""
		}
		stem := s[:200-len(ext)]
		for !utf8.ValidString(stem) {
			stem = stem[:len(stem)-1]
		}
		s = stem + ext
	}
	return s
}

// validName reports whether name refers to a visible file directly inside
// the folder.
func validName(name string) bool {
	return name != "" && !strings.HasPrefix(name, ".") && !strings.ContainsAny(name, `/\`)
}

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}
