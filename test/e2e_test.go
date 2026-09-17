//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

const (
	childText    = "Child from the smoke test"
	secondText   = "Second child after an undo"
	markdownHead = "Heading from the smoke test"
)

var chromeCandidates = []string{
	"/opt/pw-browsers/chromium-1194/chrome-linux/chrome",
	"/usr/bin/chromium",
	"/usr/bin/chromium-browser",
	"/usr/bin/google-chrome",
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
}

func TestCoreLoop(t *testing.T) {
	binary := binaryPath(t)
	chrome := chromePath(t)
	dataDir := t.TempDir()
	downloads := t.TempDir()
	base := startServer(t, binary, dataDir)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chrome),
		chromedp.NoSandbox,
		chromedp.WindowSize(1440, 900),
	)
	alloc, cancelAlloc := chromedp.NewExecAllocator(t.Context(), opts...)
	defer cancelAlloc()
	ctx, cancelCtx := chromedp.NewContext(alloc)
	defer cancelCtx()
	ctx, cancelTimeout := context.WithTimeout(ctx, 150*time.Second)
	defer cancelTimeout()

	var mu sync.Mutex
	var pageErrors []string
	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			if e.Type != runtime.APITypeError {
				return
			}
			mu.Lock()
			pageErrors = append(pageErrors, fmt.Sprint(e.Args))
			mu.Unlock()
		case *runtime.EventExceptionThrown:
			mu.Lock()
			pageErrors = append(pageErrors, e.ExceptionDetails.Text)
			mu.Unlock()
		}
	})

	if err := chromedp.Run(ctx,
		browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorAllow).WithDownloadPath(downloads),
		chromedp.Navigate(base),
		chromedp.WaitVisible(`#nodes .node`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("the app did not render the seeded sample map: %v", err)
	}

	if err := chromedp.Run(ctx,
		chromedp.Click(`#new-map`, chromedp.ByQuery),
		chromedp.WaitVisible(`#dialog[open] #dialog-input`, chromedp.ByQuery),
		chromedp.SendKeys(`#dialog-input`, "Smoke test map", chromedp.ByQuery),
		chromedp.Click(`#dialog-confirm`, chromedp.ByQuery),
		waitFor(ctx, `document.querySelectorAll('#nodes .node').length === 1`),
		chromedp.KeyEvent(kb.Escape),
	); err != nil {
		t.Fatalf("creating a map through the dialog failed: %v", err)
	}

	if err := chromedp.Run(ctx,
		chromedp.Click(`#nodes .node`, chromedp.ByQuery),
		chromedp.KeyEvent(kb.Tab),
		waitFor(ctx, `document.querySelectorAll('#nodes .node').length === 2`),
		chromedp.KeyEvent(childText),
		chromedp.KeyEvent(kb.Enter),
	); err != nil {
		t.Fatalf("adding and naming a child node failed: %v", err)
	}

	waitForDisk(t, dataDir, childText)

	// Undoing past the save above must still save, so the version token cannot live inside the undo snapshot.
	if err := chromedp.Run(ctx,
		chromedp.KeyEvent("z", chromedp.KeyModifiers(input.ModifierCtrl)),
		chromedp.KeyEvent("z", chromedp.KeyModifiers(input.ModifierCtrl)),
		waitFor(ctx, `document.querySelectorAll('#nodes .node').length === 1`),
		chromedp.KeyEvent(kb.Tab),
		waitFor(ctx, `document.querySelectorAll('#nodes .node').length === 2`),
		chromedp.KeyEvent(secondText),
		chromedp.KeyEvent(kb.Enter),
	); err != nil {
		t.Fatalf("undoing and adding a new child failed: %v", err)
	}

	waitForDisk(t, dataDir, secondText)
	if strings.Contains(readSavedMap(t, dataDir), childText) {
		t.Fatalf("the undone node %q is still on disk after a later save", childText)
	}

	if err := chromedp.Run(ctx,
		chromedp.Click(`#insp-note`, chromedp.ByQuery),
		chromedp.SendKeys(`#insp-note`, "# "+markdownHead+"\n\nBody text.", chromedp.ByQuery),
		chromedp.Click(`#nodes .node[aria-selected="true"]`, chromedp.ByQuery),
		chromedp.Click(`#nodes .node[aria-selected="true"] [data-role="note"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#md[open] #md-body h1`, chromedp.ByQuery),
		waitFor(ctx, fmt.Sprintf(`document.querySelector('#md-body h1').textContent === %q`, markdownHead)),
		chromedp.KeyEvent(kb.Escape),
		waitFor(ctx, `!document.querySelector('#md').open`),
	); err != nil {
		t.Fatalf("writing Markdown on a node and opening it rendered failed: %v", err)
	}

	waitForDisk(t, dataDir, markdownHead)

	if err := chromedp.Run(ctx,
		chromedp.Reload(),
		chromedp.WaitVisible(`#nodes .node`, chromedp.ByQuery),
		waitFor(ctx, fmt.Sprintf(`[...document.querySelectorAll('#nodes .node')].some(n => n.textContent.includes(%q))`, secondText)),
	); err != nil {
		t.Fatalf("the child node did not survive a reload, so autosave or persistence is broken: %v", err)
	}

	if err := chromedp.Run(ctx,
		chromedp.Click(`#btn-export`, chromedp.ByQuery),
		chromedp.WaitVisible(`#export-menu .export-item`, chromedp.ByQuery),
		chromedp.Click(`#export-menu .export-item[data-format="svg"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("exporting SVG from the toolbar failed: %v", err)
	}

	svg := waitForDownload(t, downloads, ".svg")
	if !strings.Contains(svg, "<svg") || !strings.Contains(textOf(svg), secondText) {
		t.Fatalf("the exported SVG does not carry the node text, got %d bytes", len(svg))
	}

	saved := readSavedMap(t, dataDir)
	if !strings.Contains(saved, secondText) {
		t.Fatalf("no map file on disk holds the new node text")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(pageErrors) > 0 {
		t.Fatalf("the page reported errors during the core loop: %s", strings.Join(pageErrors, " | "))
	}
}

// Wrapped node text is split across one tspan per line, so a tag boundary is a word boundary.
func textOf(svg string) string {
	var out strings.Builder
	depth := 0
	for _, r := range svg {
		switch {
		case r == '<':
			depth++
			out.WriteRune(' ')
		case r == '>':
			depth--
		case depth == 0:
			out.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

func waitFor(parent context.Context, expr string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			var ok bool
			if err := chromedp.Evaluate(expr, &ok).Do(ctx); err == nil && ok {
				return nil
			}
			select {
			case <-parent.Done():
				return parent.Err()
			case <-time.After(150 * time.Millisecond):
			}
		}
		return fmt.Errorf("the page never satisfied %s", expr)
	})
}

func waitForDisk(t *testing.T, dataDir, want string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(readSavedMap(t, dataDir), want) {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("autosave never wrote %q to a map file in %s", want, dataDir)
}

func binaryPath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "inoichi"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("no inoichi binary at %s, run 'make build' first", p)
	}
	return p
}

func chromePath(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("INOICHI_CHROME"); p != "" {
		return p
	}
	for _, c := range chromeCandidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("no Chrome or Chromium found, set INOICHI_CHROME to its path")
	return ""
}

func startServer(t *testing.T, binary, dataDir string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	cmd := exec.Command(binary, "serve", "--host", "127.0.0.1", "--port", fmt.Sprint(port), "--data-dir", dataDir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("could not start the server: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if healthy(base) {
			return base
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("the server never answered on %s", base)
	return ""
}

func healthy(base string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func waitForDownload(t *testing.T, dir, suffix string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), suffix) {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err == nil && len(data) > 0 {
				return string(data)
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("no %s file appeared in %s", suffix, dir)
	return ""
}

func readSavedMap(t *testing.T, dataDir string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dataDir, "maps"))
	if err != nil {
		t.Fatalf("could not read the data directory: %v", err)
	}
	var all strings.Builder
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dataDir, "maps", e.Name()))
		if err == nil {
			all.Write(data)
		}
	}
	return all.String()
}
