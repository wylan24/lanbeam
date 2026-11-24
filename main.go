// LanBeam moves files between an iPhone and a PC over the local network.
// Run it on the PC, scan the QR code with the iPhone camera, done.
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var version = "dev"

func main() {
	home, _ := os.UserHomeDir()
	dir := flag.String("dir", filepath.Join(home, "Downloads", "LanBeam"), "folder where transferred files are kept")
	port := flag.Int("port", 8765, "port to listen on (0 picks any free port)")
	noBrowser := flag.Bool("no-browser", false, "don't open the control page on this computer")
	newToken := flag.Bool("new-token", false, "create a new access key; phones must scan the QR code again")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("lanbeam", version)
		return
	}

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		fatal("cannot create folder %s: %v", *dir, err)
	}
	token := loadToken(*newToken)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil && *port != 0 {
		ln, err = net.Listen("tcp", ":0")
	}
	if err != nil {
		fatal("cannot start server: %v", err)
	}
	actual := ln.Addr().(*net.TCPAddr).Port

	var urls []string
	for _, ip := range lanIPs() {
		urls = append(urls, fmt.Sprintf("http://%s:%d/?t=%s", ip, actual, token))
	}
	local := fmt.Sprintf("http://localhost:%d/?t=%s", actual, token)

	fmt.Printf("\n  LanBeam %s\n  Files are saved in: %s\n\n", version, *dir)
	if len(urls) == 0 {
		fmt.Println("  No network found. Connect this computer to Wi-Fi (or your iPhone's hotspot) and restart.")
	} else {
		printQR(os.Stdout, urls[0])
		fmt.Printf("\n  Scan the code with your iPhone camera, or open:\n  %s\n", urls[0])
		for _, u := range urls[1:] {
			fmt.Printf("  (other address: %s)\n", u)
		}
	}
	fmt.Println("\n  Keep this window open while transferring. Press Ctrl+C to quit.")

	if !*noBrowser {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openPath(local)
		}()
	}

	srv := &http.Server{
		Handler:           newServer(*dir, token, urls),
		ReadHeaderTimeout: 15 * time.Second,
	}
	if err := srv.Serve(ln); err != nil {
		fatal("server stopped: %v", err)
	}
}

// loadToken returns the access key, kept in the user config dir so that a
// bookmark or home-screen shortcut on the phone keeps working across runs.
func loadToken(rotate bool) string {
	var path string
	if cfg, err := os.UserConfigDir(); err == nil {
		path = filepath.Join(cfg, "lanbeam", "token")
		if b, err := os.ReadFile(path); err == nil && !rotate {
			if t := strings.TrimSpace(string(b)); len(t) >= 16 {
				return t
			}
		}
	}
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		fatal("cannot generate access key: %v", err)
	}
	t := hex.EncodeToString(b)
	if path != "" {
		_ = os.MkdirAll(filepath.Dir(path), 0o700)
		_ = os.WriteFile(path, []byte(t), 0o600)
	}
	return t
}

// openPath opens a URL or folder with the system's default handler.
func openPath(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if strings.HasPrefix(target, "http") {
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
		} else {
			cmd = exec.Command("explorer", target)
		}
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

// fatal prints the error and, on Windows, waits for Enter so a double-clicked
// console window doesn't vanish before the message can be read.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\n  Error: "+format+"\n", args...)
	if runtime.GOOS == "windows" {
		fmt.Fprint(os.Stderr, "\n  Press Enter to close.")
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
	os.Exit(1)
}
