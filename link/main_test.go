package main

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeSocks is a minimal SOCKS5 server: it accepts one CONNECT and then echoes, recording the
// address it was asked for.
func fakeSocks(t *testing.T, replyCode byte) (addr string, asked chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	asked = make(chan string, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				hello := make([]byte, 3)
				if _, err := io.ReadFull(c, hello); err != nil {
					return
				}
				c.Write([]byte{5, 0})
				head := make([]byte, 4)
				if _, err := io.ReadFull(c, head); err != nil {
					return
				}
				var host string
				switch head[3] {
				case 1:
					b := make([]byte, 4)
					io.ReadFull(c, b)
					host = net.IP(b).String()
				case 3:
					n := make([]byte, 1)
					io.ReadFull(c, n)
					b := make([]byte, n[0])
					io.ReadFull(c, b)
					host = string(b)
				}
				p := make([]byte, 2)
				io.ReadFull(c, p)
				asked <- net.JoinHostPort(host, itoa(int(p[0])<<8|int(p[1])))
				c.Write([]byte{5, replyCode, 0, 1, 0, 0, 0, 0, 0, 0})
				if replyCode == 0 {
					io.Copy(c, c)
				}
			}()
		}
	}()
	return ln.Addr().String(), asked
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func TestForwardReachesTheTargetThroughTheProxy(t *testing.T) {
	proxy, asked := fakeSocks(t, 0)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go newForwarder(proxy, "10.20.30.40:443").serve(ln)

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 5)
	if _, err := io.ReadFull(c, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("echo = %q", got)
	}
	if a := <-asked; a != "10.20.30.40:443" {
		t.Fatalf("proxy was asked for %q", a)
	}
}

func TestHostNamesAreSentAsNames(t *testing.T) {
	proxy, asked := fakeSocks(t, 0)
	c, err := dialSocks5(proxy, "books.example:443", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if a := <-asked; a != "books.example:443" {
		t.Fatalf("proxy was asked for %q", a)
	}
}

func TestProxyRefusalIsAnError(t *testing.T) {
	proxy, _ := fakeSocks(t, 5)
	if _, err := dialSocks5(proxy, "10.20.30.40:443", time.Second); err == nil {
		t.Fatal("expected an error when the proxy cannot connect")
	}
}

func TestDeadProxyIsAnError(t *testing.T) {
	if _, err := dialSocks5("127.0.0.1:1", "10.20.30.40:443", time.Second); err == nil {
		t.Fatal("expected an error when the proxy is not listening")
	}
}

func TestProbeReportsWhetherTheTargetIsReachable(t *testing.T) {
	ok, _ := fakeSocks(t, 0)
	if err := probe(ok, "10.20.30.40:443", time.Second); err != nil {
		t.Fatalf("probe through a working proxy: %v", err)
	}
	refused, _ := fakeSocks(t, 5)
	if err := probe(refused, "10.20.30.40:443", time.Second); err == nil {
		t.Fatal("probe must fail when the proxy cannot reach the target")
	}
	if err := probe("127.0.0.1:1", "10.20.30.40:443", time.Second); err == nil {
		t.Fatal("probe must fail when there is no proxy")
	}
}

func TestTargetsThatWouldLoopAreRefused(t *testing.T) {
	for _, bad := range []string{"127.0.0.1:1080", "127.5.5.5:443", "[::1]:443", "0.0.0.0:443", "localhost:443", "10.0.0.1", "10.0.0.1:0", "10.0.0.1:70000", ":443"} {
		if err := checkTarget(bad); err == nil {
			t.Errorf("checkTarget(%q) = nil, want an error", bad)
		}
	}
	for _, good := range []string{"10.20.30.40:443", "192.168.1.10:8080", "books.example:443"} {
		if err := checkTarget(good); err != nil {
			t.Errorf("checkTarget(%q) = %v", good, err)
		}
	}
}

func TestAProxyThatSpeaksAnotherVersionIsAnError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		io.ReadFull(c, make([]byte, 3))
		c.Write([]byte{5, 0})
		io.ReadFull(c, make([]byte, 10))
		c.Write([]byte{4, 0, 0, 1, 0, 0, 0, 0, 0, 0}) // version 4 in the CONNECT reply
	}()
	if _, err := dialSocks5(ln.Addr().String(), "10.20.30.40:443", time.Second); err == nil {
		t.Fatal("expected an error for a reply that is not SOCKS version 5")
	}
}

func TestConnectionsBeyondTheCapAreClosed(t *testing.T) {
	proxy, _ := fakeSocks(t, 0)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	f := newForwarder(proxy, "10.20.30.40:443")
	f.slots = make(chan struct{}, 1)
	go f.serve(ln)

	first, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	first.Write([]byte("a"))
	if _, err := io.ReadFull(first, make([]byte, 1)); err != nil {
		t.Fatalf("first connection should work: %v", err)
	}

	second, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	second.SetReadDeadline(time.Now().Add(2 * time.Second))
	second.Write([]byte("b"))
	if _, err := io.ReadFull(second, make([]byte, 1)); err == nil {
		t.Fatal("a connection beyond the cap must be closed, not served")
	}
}

func TestAnIdleConnectionIsClosed(t *testing.T) {
	proxy, _ := fakeSocks(t, 0)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	f := newForwarder(proxy, "10.20.30.40:443")
	f.idle = 150 * time.Millisecond
	go f.serve(ln)

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Write([]byte("a"))
	if _, err := io.ReadFull(c, make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	start := time.Now()
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("an idle connection should be closed by the forwarder")
	}
	if waited := time.Since(start); waited > 2*time.Second {
		t.Fatalf("idle close took %v, want about 150ms", waited)
	}
}

func TestCaptureKeepsALogUnderItsCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.log")
	var in bytes.Buffer
	for i := 0; i < 400; i++ {
		in.WriteString(strings.Repeat("x", 99) + "\n")
	}
	in.WriteString("the last line\n")
	if err := capture(&in, path, 4096); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 4096 {
		t.Fatalf("log is %d bytes, cap is 4096", len(got))
	}
	if !strings.HasSuffix(string(got), "the last line\n") {
		t.Fatal("the newest output must survive the trim")
	}
	if len(got) < 1024 {
		t.Fatalf("log is only %d bytes: the trim threw away too much", len(got))
	}
}

// streamingSocks accepts one CONNECT and then sends a chunk every interval for total, never
// reading: the shape of a download.
func streamingSocks(t *testing.T, interval, total time.Duration) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				io.ReadFull(c, make([]byte, 3))
				c.Write([]byte{5, 0})
				io.ReadFull(c, make([]byte, 10))
				c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
				end := time.Now().Add(total)
				for time.Now().Before(end) {
					if _, err := c.Write([]byte("chunk")); err != nil {
						return
					}
					time.Sleep(interval)
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func TestADownloadLongerThanTheIdleTimeoutIsNotCutOff(t *testing.T) {
	proxy := streamingSocks(t, 40*time.Millisecond, 900*time.Millisecond)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	f := newForwarder(proxy, "10.20.30.40:443")
	f.idle = 150 * time.Millisecond // the client stays silent for six times this long
	go f.serve(ln)

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	start := time.Now()
	n, _ := io.Copy(io.Discard, c)
	took := time.Since(start)
	if took < 800*time.Millisecond {
		t.Fatalf("stream ended after %v and %d bytes: a healthy one-way transfer was cut off", took, n)
	}
	if n < 15*5 {
		t.Fatalf("received only %d bytes of the stream", n)
	}
}

func TestListenersMustBeLoopback(t *testing.T) {
	for _, bad := range []string{"0.0.0.0:14431", "192.168.1.5:14431", ":14431", "localhost:14431", "14431"} {
		if err := checkListen(bad); err == nil {
			t.Errorf("checkListen(%q) = nil, want an error", bad)
		}
	}
	for _, good := range []string{"127.0.0.1:14431", "[::1]:14431"} {
		if err := checkListen(good); err != nil {
			t.Errorf("checkListen(%q) = %v", good, err)
		}
	}
}

func TestCaptureSurvivesAChunkBiggerThanTheCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.log")
	in := strings.NewReader(strings.Repeat("y", 20000) + "\nlast\n")
	if err := capture(in, path, 2048); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if len(got) > 2048 || !strings.HasSuffix(string(got), "last\n") {
		t.Fatalf("log is %d bytes and ends %q", len(got), string(got[max(0, len(got)-10):]))
	}
}

func TestDialReportsWhetherAnAddressAccepts(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	if err := dial(addr, time.Second); err != nil {
		t.Fatalf("dial to a listener: %v", err)
	}
	ln.Close()
	if err := dial(addr, time.Second); err == nil {
		t.Fatal("dial to a closed port must fail")
	}
}
