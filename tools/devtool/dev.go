package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

// devApp is one React app served by its own Vite dev server with hot reload.
type devApp struct {
	name string // workspace directory under web/apps
	path string // URL prefix the app is served under (its Vite base)
	port int    // preferred port; the next free one is used when it is taken
}

// devApps start well above Vite's default 5173 range, which other local
// projects commonly occupy.
var devApps = []devApp{
	{"realization", "/", 5191},
	{"financing", "/finance/", 5192},
	{"insurance", "/insurance/", 5193},
	{"admin", "/admin/", 5194},
}

// devPortSearch bounds how far past a taken preferred port dev looks.
const devPortSearch = 50

// defaultAPIAddr matches the Makefile default when HTTP_ADDR is unset.
const defaultAPIAddr = "127.0.0.1:8080"

// runDev runs the API and the four Vite dev servers with hot reload until
// Ctrl-C or until any of them exits, then stops the rest. The database must
// already be up and migrated (`make dev` does that first). Vite origins are
// added to the API's ALLOWED_ORIGINS for this run only, so .env needs no edit.
func runDev(ctx context.Context, a *app, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("takes no arguments, got %q", strings.Join(args, " "))
	}
	portFree := a.portFree
	if portFree == nil {
		portFree = tcpPortFree
	}
	ports, err := pickDevPorts(portFree)
	if err != nil {
		return err
	}
	apiAddr := a.getenv("HTTP_ADDR")
	if apiAddr == "" {
		apiAddr = defaultAPIAddr
	}

	origins := make([]string, 0, len(devApps)+1)
	if existing := a.getenv("ALLOWED_ORIGINS"); existing != "" {
		origins = append(origins, existing)
	}
	for i := range devApps {
		origins = append(origins, "http://127.0.0.1:"+strconv.Itoa(ports[i]))
	}

	fmt.Fprintf(a.stdout, "API          http://%s\n", apiAddr)
	for i, app := range devApps {
		fmt.Fprintf(a.stdout, "%-12s http://127.0.0.1:%d%s\n", app.name, ports[i], app.path)
	}
	fmt.Fprintln(a.stdout, "Hot reload is on; Ctrl-C stops everything.")

	// Later entries win in os/exec, so these override inherited values.
	// WEB_DIR is emptied: the API serves only /api, Vite serves the apps.
	apiEnv := append(os.Environ(), "ALLOWED_ORIGINS="+strings.Join(origins, ","), "WEB_DIR=")
	webEnv := append(os.Environ(), "JUSTIX_API=http://"+apiAddr)

	procs := []struct {
		label string
		cmd   command
	}{{"api", command{
		name: "bash", args: []string{a.root + "/tools/go.sh", "run", "./cmd/api"},
		dir: a.root, env: apiEnv,
	}}}
	for i, app := range devApps {
		procs = append(procs, struct {
			label string
			cmd   command
		}{app.name, command{
			name: "npm",
			args: []string{"run", "dev", "--workspace", "web/apps/" + app.name, "--", "--port", strconv.Itoa(ports[i]), "--strictPort"},
			dir:  a.root, env: webEnv,
		}})
	}

	runCtx, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	var mu sync.Mutex // one output line at a time across all processes
	type ended struct {
		label string
		code  int
		err   error
	}
	endedCh := make(chan ended, len(procs))
	for _, p := range procs {
		out := &prefixWriter{w: a.stdout, prefix: "[" + p.label + "] ", mu: &mu}
		errOut := &prefixWriter{w: a.stderr, prefix: "[" + p.label + "] ", mu: &mu}
		p.cmd.stdout, p.cmd.stderr = out, errOut
		go func() {
			_, code, err := a.run.run(runCtx, p.cmd)
			out.flush()
			errOut.flush()
			endedCh <- ended{p.label, code, err}
		}()
	}

	first := <-endedCh
	stop(fmt.Errorf("%s exited", first.label))
	for range len(procs) - 1 {
		<-endedCh
	}
	if ctx.Err() != nil {
		return context.Cause(ctx) // Ctrl-C: realMain maps it to 130/143
	}
	if first.err != nil {
		return fmt.Errorf("%s could not run: %w; stopped the others", first.label, first.err)
	}
	return fmt.Errorf("%s exited with code %d; stopped the others", first.label, first.code)
}

// pickDevPorts returns one free port per dev app, starting at its preferred
// port and never handing the same port to two apps.
func pickDevPorts(portFree func(int) bool) ([]int, error) {
	used := map[int]bool{}
	ports := make([]int, len(devApps))
	for i, app := range devApps {
		found := false
		for p := app.port; p < app.port+devPortSearch; p++ {
			if !used[p] && portFree(p) {
				ports[i], used[p], found = p, true, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("no free port for %s in %d-%d", app.name, app.port, app.port+devPortSearch-1)
		}
	}
	return ports, nil
}

// tcpPortFree reports whether 127.0.0.1:port can be listened on right now.
func tcpPortFree(port int) bool {
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	return l.Close() == nil
}

// prefixWriter writes complete lines to w with a prefix, holding a partial
// line until its newline (or flush) so output from several processes does
// not interleave mid-line.
type prefixWriter struct {
	w      io.Writer
	prefix string
	mu     *sync.Mutex
	buf    []byte
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	p.buf = append(p.buf, b...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			return len(b), nil
		}
		if err := p.emit(p.buf[:i+1]); err != nil {
			return len(b), err
		}
		p.buf = p.buf[i+1:]
	}
}

func (p *prefixWriter) flush() {
	if len(p.buf) > 0 {
		_ = p.emit(append(p.buf, '\n'))
		p.buf = nil
	}
}

func (p *prefixWriter) emit(line []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := io.WriteString(p.w, p.prefix+string(line))
	if err != nil && !errors.Is(err, io.ErrClosedPipe) {
		return err
	}
	return nil
}
