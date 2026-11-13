// nblink forwards local TCP ports to fixed addresses through a SOCKS5 proxy.
//
// NetBird in userspace ("netstack") mode has no network interface: the only way in is the
// SOCKS5 proxy it listens on. Most programs on a Kindle cannot speak SOCKS5. nblink bridges the
// two: a firewall rule redirects connections for a mesh address to a local port, nblink accepts
// them there and opens the real connection through the proxy.
//
//	nblink -socks 127.0.0.1:1080 -forward 127.0.0.1:14431=10.20.30.40:443 [-forward ...]
//	nblink -socks 127.0.0.1:1080 -probe 10.20.30.40:443     exit 0 if the target answers
//	nblink -dial 10.20.30.40:443                             exit 0 if a plain connection opens
//	some-command 2>&1 | nblink -capture FILE -max 262144      keep a log under a size cap
package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	dialTimeout = 20 * time.Second
	idleTimeout = 5 * time.Minute
	maxConns    = 64
	logEvery    = 30 * time.Second
)

type forwards []string

func (f *forwards) String() string     { return strings.Join(*f, ",") }
func (f *forwards) Set(v string) error { *f = append(*f, v); return nil }

func main() {
	var fw forwards
	socks := flag.String("socks", "127.0.0.1:1080", "SOCKS5 proxy address")
	flag.Var(&fw, "forward", "LISTEN=TARGET, e.g. 127.0.0.1:14431=10.20.30.40:443 (repeatable)")
	probeTarget := flag.String("probe", "", "check that TARGET answers through the proxy, then exit")
	dialTarget := flag.String("dial", "", "check that a plain TCP connection to ADDR opens, then exit")
	timeout := flag.Duration("timeout", dialTimeout, "how long -probe and -dial wait")
	capturePath := flag.String("capture", "", "copy standard input to this file, keeping it under -max bytes")
	captureMax := flag.Int64("max", 256*1024, "size cap for -capture and -log, in bytes")
	logPath := flag.String("log", "", "write the forwarder's own log to this file, under -max bytes")
	flag.Parse()

	switch {
	case *capturePath != "":
		if err := capture(os.Stdin, *capturePath, *captureMax); err != nil {
			log.Fatalf("nblink: %v", err)
		}
		return
	case *probeTarget != "":
		if err := checkTarget(*probeTarget); err != nil {
			log.Fatalf("nblink: %v", err)
		}
		if err := probe(*socks, *probeTarget, *timeout); err != nil {
			fmt.Fprintln(os.Stderr, "nblink:", err)
			os.Exit(1)
		}
		return
	case *dialTarget != "":
		if err := dial(*dialTarget, *timeout); err != nil {
			fmt.Fprintln(os.Stderr, "nblink:", err)
			os.Exit(1)
		}
		return
	}

	if *logPath != "" {
		out, err := openCapped(*logPath, *captureMax)
		if err != nil {
			log.Fatalf("nblink: %v", err)
		}
		log.SetOutput(out)
	}

	if len(fw) == 0 {
		log.Fatal("nblink: at least one -forward LISTEN=TARGET is required")
	}
	done := make(chan error, len(fw))
	for _, pair := range fw {
		listen, target, ok := strings.Cut(pair, "=")
		if !ok {
			log.Fatalf("nblink: bad -forward %q, want LISTEN=TARGET", pair)
		}
		if err := checkTarget(target); err != nil {
			log.Fatalf("nblink: %v", err)
		}
		if err := checkListen(listen); err != nil {
			log.Fatalf("nblink: %v", err)
		}
		ln, err := net.Listen("tcp", listen)
		if err != nil {
			log.Fatalf("nblink: %v", err)
		}
		log.Printf("nblink: %s -> %s via socks5 %s", listen, target, *socks)
		f := newForwarder(*socks, target)
		go func() { done <- f.serve(ln) }()
	}
	log.Fatal(<-done)
}

// checkTarget refuses targets that cannot be right. A loopback or unspecified address would
// send the connection straight back into the redirect that delivered it.
func checkTarget(target string) error {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("bad target %q: %v", target, err)
	}
	if port, err := strconv.Atoi(portStr); err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("bad target %q: port must be 1-65535", target)
	}
	if host == "" || strings.EqualFold(host, "localhost") {
		return fmt.Errorf("bad target %q: a target on this device would loop", target)
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsUnspecified()) {
		return fmt.Errorf("bad target %q: a target on this device would loop", target)
	}
	return nil
}

// checkListen keeps the listeners on loopback: nothing off the device should reach the mesh
// through this program.
func checkListen(listen string) error {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("bad listen address %q: %v", listen, err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("bad listen address %q: only loopback addresses are allowed", listen)
	}
	return nil
}

// probe reports whether target answers through the proxy.
func probe(socks, target string, timeout time.Duration) error {
	c, err := dialSocks5(socks, target, timeout)
	if err != nil {
		return err
	}
	return c.Close()
}

// dial reports whether a plain TCP connection to addr opens. With the firewall redirect in
// place this shows that the redirect and the forwarder behind it are both there.
func dial(addr string, timeout time.Duration) error {
	c, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	return c.Close()
}

type forwarder struct {
	socks, target string
	idle          time.Duration
	slots         chan struct{}

	mu       sync.Mutex
	lastLog  time.Time
	dropped  int
	failures int
}

func newForwarder(socks, target string) *forwarder {
	return &forwarder{socks: socks, target: target, idle: idleTimeout, slots: make(chan struct{}, maxConns)}
}

func (f *forwarder) serve(ln net.Listener) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() || isTemporary(err) {
				time.Sleep(200 * time.Millisecond) // out of descriptors, say: wait, do not exit
				continue
			}
			return err
		}
		select {
		case f.slots <- struct{}{}:
			go func() {
				defer func() { <-f.slots }()
				f.handle(c)
			}()
		default:
			c.Close()
			f.note(&f.dropped, "connections refused: too many at once")
		}
	}
}

func isTemporary(err error) bool {
	var t interface{ Temporary() bool }
	return errors.As(err, &t) && t.Temporary()
}

// note counts an event and logs the count at most once per logEvery, so a client retrying
// against a dead link cannot fill the log.
func (f *forwarder) note(counter *int, what string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	*counter++
	if time.Since(f.lastLog) < logEvery {
		return
	}
	log.Printf("nblink: %s: %s (%d since last report)", f.target, what, *counter)
	*counter = 0
	f.lastLog = time.Now()
}

// activity is the moment a connection last carried data in either direction.
type activity struct{ last atomic.Int64 }

func (a *activity) touch()                  { a.last.Store(time.Now().UnixNano()) }
func (a *activity) quietFor() time.Duration { return time.Since(time.Unix(0, a.last.Load())) }

func (f *forwarder) handle(client net.Conn) {
	defer client.Close()
	up, err := dialSocks5(f.socks, f.target, dialTimeout)
	if err != nil {
		f.note(&f.failures, err.Error())
		return
	}
	defer up.Close()

	act := &activity{}
	act.touch()
	var wg sync.WaitGroup
	run := func(dst, src net.Conn) {
		defer wg.Done()
		if !f.pipe(dst, src, act) {
			// an error or an idle connection ends both directions
			client.Close()
			up.Close()
		}
	}
	wg.Add(2)
	go run(up, client)
	go run(client, up)
	wg.Wait()
}

// pipe copies src to dst. It returns true when src ended cleanly, having passed the end on to
// dst so the other direction can finish by itself; false on an error, or when the connection
// has been silent in both directions for the idle timeout. A download, where one side sends
// nothing for a long time, is not idle.
func (f *forwarder) pipe(dst, src net.Conn, act *activity) bool {
	tick := f.idle / 3
	if tick > 30*time.Second {
		tick = 30 * time.Second
	}
	if tick < 10*time.Millisecond {
		tick = 10 * time.Millisecond
	}
	buf := make([]byte, 16*1024)
	for {
		src.SetReadDeadline(time.Now().Add(tick))
		n, err := src.Read(buf)
		if n > 0 {
			act.touch()
			dst.SetWriteDeadline(time.Now().Add(f.idle))
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return false
			}
		}
		if err == nil {
			continue
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			if act.quietFor() >= f.idle {
				return false
			}
			continue
		}
		if errors.Is(err, io.EOF) {
			if t, ok := dst.(*net.TCPConn); ok {
				t.CloseWrite()
			}
			return true
		}
		return false
	}
}

// dialSocks5 opens a TCP connection to target through a SOCKS5 proxy, no authentication.
func dialSocks5(proxy, target string, timeout time.Duration) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("bad port %q", portStr)
	}
	c, err := net.DialTimeout("tcp", proxy, timeout)
	if err != nil {
		return nil, fmt.Errorf("proxy not reachable: %w", err)
	}
	c.SetDeadline(time.Now().Add(timeout))
	fail := func(err error) (net.Conn, error) { c.Close(); return nil, err }

	// greeting: version 5, one method, "no authentication"
	if _, err := c.Write([]byte{5, 1, 0}); err != nil {
		return fail(err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(c, reply); err != nil {
		return fail(err)
	}
	if reply[0] != 5 || reply[1] != 0 {
		return fail(errors.New("proxy refused the no-authentication method"))
	}

	// CONNECT request
	req := []byte{5, 1, 0}
	if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
		req = append(append(req, 1), ip.To4()...)
	} else if ip != nil {
		req = append(append(req, 4), ip.To16()...)
	} else {
		if len(host) > 255 {
			return fail(errors.New("host name too long"))
		}
		req = append(append(req, 3, byte(len(host))), host...)
	}
	req = binary.BigEndian.AppendUint16(req, uint16(port))
	if _, err := c.Write(req); err != nil {
		return fail(err)
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(c, head); err != nil {
		return fail(err)
	}
	if head[0] != 5 {
		return fail(fmt.Errorf("proxy answered with version %d, not SOCKS 5", head[0]))
	}
	if head[1] != 0 {
		return fail(fmt.Errorf("proxy could not connect (code %d)", head[1]))
	}
	var skip int
	switch head[3] {
	case 1:
		skip = 4
	case 4:
		skip = 16
	case 3:
		n := make([]byte, 1)
		if _, err := io.ReadFull(c, n); err != nil {
			return fail(err)
		}
		skip = int(n[0])
	default:
		return fail(errors.New("proxy sent an unknown address type"))
	}
	if _, err := io.ReadFull(c, make([]byte, skip+2)); err != nil {
		return fail(err)
	}
	c.SetDeadline(time.Time{})
	return c, nil
}

// cappedFile is a log file that never grows past max bytes: when a write would take it over,
// the older half is dropped first. The Kindle has no log rotation and its writable partitions
// are small.
type cappedFile struct {
	mu   sync.Mutex
	f    *os.File
	size int64
	max  int64
}

func openCapped(path string, max int64) (*cappedFile, error) {
	if max < 1024 {
		max = 1024
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	return &cappedFile{f: f, max: max}, nil
}

func (c *cappedFile) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	chunk := p
	if int64(len(chunk)) > c.max/2 {
		chunk = chunk[int64(len(chunk))-c.max/2:] // one write bigger than the cap: keep its end
	}
	if c.size+int64(len(chunk)) > c.max {
		if err := c.trim(c.max / 2); err != nil {
			return 0, err
		}
	}
	n, err := c.f.Write(chunk)
	c.size += int64(n)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// trim keeps the last keep bytes of the file, from the start of a line where it can.
func (c *cappedFile) trim(keep int64) error {
	if keep > c.size {
		keep = c.size
	}
	tail := make([]byte, keep)
	if _, err := c.f.ReadAt(tail, c.size-keep); err != nil && err != io.EOF {
		return err
	}
	if i := strings.IndexByte(string(tail), '\n'); i >= 0 && i+1 < len(tail) {
		tail = tail[i+1:]
	}
	if err := c.f.Truncate(0); err != nil {
		return err
	}
	if _, err := c.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	n, err := c.f.Write(tail)
	c.size = int64(n)
	return err
}

// capture copies r to a capped file at path until r ends. If the file cannot be written (a
// full tmpfs, say) it keeps reading and throws the data away: the program on the other end of
// the pipe must never be killed by its own log.
func capture(r io.Reader, path string, max int64) error {
	out, err := openCapped(path, max)
	if err != nil {
		_, _ = io.Copy(io.Discard, r)
		return err
	}
	defer out.f.Close()
	buf := make([]byte, 8*1024)
	writable := true
	for {
		n, rerr := r.Read(buf)
		if n > 0 && writable {
			if _, werr := out.Write(buf[:n]); werr != nil {
				writable = false
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}
