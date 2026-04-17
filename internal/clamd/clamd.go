// Package clamd implements a small client for the ClamAV daemon
// protocol over TCP or Unix sockets. See THIRD_PARTY_LICENSES.md for
// license attribution.
package clamd

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// Result status constants returned by clamd.
const (
	ResOK         = "OK"
	ResFound      = "FOUND"
	ResError      = "ERROR"
	ResParseError = "PARSE ERROR"
)

// Clamd is a handle to a running clamd daemon reachable over TCP or Unix
// socket.
type Clamd struct {
	address string
}

// Stats reports daemon queue/pool/memory status.
type Stats struct {
	Pools    string
	State    string
	Threads  string
	Memstats string
	Queue    string
}

// ScanResult is one parsed reply line from clamd.
type ScanResult struct {
	Raw         string
	Description string
	Path        string
	Hash        string
	Size        int
	Status      string
}

// EICAR is the standard AV test payload.
var EICAR = []byte(`X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`)

// NewClamd builds a client against a clamd address. The address is a URL
// (`tcp://host:port`, `unix:///var/run/clamav/clamd.sock`) or a plain
// Unix socket path.
func NewClamd(address string) *Clamd {
	return &Clamd{address: address}
}

func (c *Clamd) newConnection() (*CLAMDConn, error) {
	u, err := url.Parse(c.address)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "tcp":
		return newCLAMDTcpConn(u.Host)
	case "unix":
		return newCLAMDUnixConn(u.Path)
	default:
		return newCLAMDUnixConn(c.address)
	}
}

func (c *Clamd) simpleCommand(command string) (chan *ScanResult, error) {
	conn, err := c.newConnection()
	if err != nil {
		return nil, err
	}

	if err := conn.sendCommand(command); err != nil {
		_ = conn.Close()
		return nil, err
	}

	ch, wg, err := conn.readResponse()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	go func() {
		wg.Wait()
		_ = conn.Close()
	}()

	return ch, nil
}

// Ping sends PING and returns nil if the daemon replies PONG.
func (c *Clamd) Ping() error {
	ch, err := c.simpleCommand("PING")
	if err != nil {
		return err
	}
	s := <-ch
	if s == nil {
		return errors.New("clamd: empty response to PING")
	}
	if s.Raw != "PONG" {
		return fmt.Errorf("clamd: invalid PING response %q", s.Raw)
	}
	return nil
}

// Version returns the program/database version stream from clamd.
func (c *Clamd) Version() (chan *ScanResult, error) {
	return c.simpleCommand("VERSION")
}

// Stats returns scan queue, thread, and memory stats.
func (c *Clamd) Stats() (*Stats, error) {
	ch, err := c.simpleCommand("STATS")
	if err != nil {
		return nil, err
	}

	stats := &Stats{}
	for s := range ch {
		switch {
		case strings.HasPrefix(s.Raw, "POOLS"):
			stats.Pools = strings.Trim(s.Raw[6:], " ")
		case strings.HasPrefix(s.Raw, "STATE"):
			stats.State = s.Raw
		case strings.HasPrefix(s.Raw, "THREADS"):
			stats.Threads = s.Raw
		case strings.HasPrefix(s.Raw, "QUEUE"):
			stats.Queue = s.Raw
		case strings.HasPrefix(s.Raw, "MEMSTATS"):
			stats.Memstats = s.Raw
		}
	}

	return stats, nil
}

// Reload instructs clamd to reload its signature databases.
func (c *Clamd) Reload() error {
	ch, err := c.simpleCommand("RELOAD")
	if err != nil {
		return err
	}
	s := <-ch
	if s == nil {
		return errors.New("clamd: empty RELOAD response")
	}
	if s.Raw != "RELOADING" {
		return fmt.Errorf("clamd: invalid RELOAD response %q", s.Raw)
	}
	return nil
}

// Shutdown asks clamd to exit.
func (c *Clamd) Shutdown() error {
	_, err := c.simpleCommand("SHUTDOWN")
	return err
}

// ScanFile recursively scans a path with archive support.
func (c *Clamd) ScanFile(path string) (chan *ScanResult, error) {
	return c.simpleCommand(fmt.Sprintf("SCAN %s", path))
}

// RawScanFile scans without archive or special-file support.
func (c *Clamd) RawScanFile(path string) (chan *ScanResult, error) {
	return c.simpleCommand(fmt.Sprintf("RAWSCAN %s", path))
}

// MultiScanFile parallel-scans a path using multiple threads.
func (c *Clamd) MultiScanFile(path string) (chan *ScanResult, error) {
	return c.simpleCommand(fmt.Sprintf("MULTISCAN %s", path))
}

// ContScanFile scans without stopping on the first virus.
func (c *Clamd) ContScanFile(path string) (chan *ScanResult, error) {
	return c.simpleCommand(fmt.Sprintf("CONTSCAN %s", path))
}

// AllMatchScanFile reports every signature match, not just the first.
func (c *Clamd) AllMatchScanFile(path string) (chan *ScanResult, error) {
	return c.simpleCommand(fmt.Sprintf("ALLMATCHSCAN %s", path))
}

// ScanStream streams an io.Reader to clamd for scanning. `abort` can be
// closed to cancel the in-flight stream.
func (c *Clamd) ScanStream(r io.Reader, abort chan bool) (chan *ScanResult, error) {
	conn, err := c.newConnection()
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			_, ok := <-abort
			if !ok {
				break
			}
		}
		_ = conn.Close()
	}()

	if err := conn.sendCommand("INSTREAM"); err != nil {
		_ = conn.Close()
		return nil, err
	}

	buf := make([]byte, CHUNK_SIZE)
	for {
		nr, rerr := r.Read(buf)
		if nr > 0 {
			if werr := conn.sendChunk(buf[:nr]); werr != nil {
				_ = conn.Close()
				return nil, werr
			}
		}
		if rerr != nil {
			break
		}
	}

	if err := conn.sendEOF(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	ch, wg, err := conn.readResponse()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	go func() {
		wg.Wait()
		_ = conn.Close()
	}()

	return ch, nil
}
