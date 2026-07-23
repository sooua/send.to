package clamd

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CHUNK_SIZE is the byte count of each frame in the INSTREAM protocol.
const CHUNK_SIZE = 1024

// TCP_TIMEOUT bounds how long Dial waits for the daemon.
const TCP_TIMEOUT = 2 * time.Second

var resultRegex = regexp.MustCompile(
	`^(?P<path>[^:]+): ((?P<desc>[^:]+)(\((?P<virhash>([^:]+)):(?P<virsize>\d+)\))? )?(?P<status>FOUND|ERROR|OK)$`,
)

// CLAMDConn wraps a net.Conn with the framing helpers clamd expects.
type CLAMDConn struct {
	net.Conn
}

func (conn *CLAMDConn) sendCommand(command string) error {
	_, err := fmt.Fprintf(conn, "n%s\n", command)
	return err
}

func (conn *CLAMDConn) sendEOF() error {
	_, err := conn.Write([]byte{0, 0, 0, 0})
	return err
}

func (conn *CLAMDConn) sendChunk(data []byte) error {
	var buf [4]byte
	n := len(data)
	buf[0] = byte(n >> 24)
	buf[1] = byte(n >> 16)
	buf[2] = byte(n >> 8)
	buf[3] = byte(n)

	if _, err := conn.Write(buf[:]); err != nil {
		return err
	}
	_, err := conn.Write(data)
	return err
}

func (c *CLAMDConn) readResponse() (chan *ScanResult, *sync.WaitGroup, error) {
	var wg sync.WaitGroup
	wg.Add(1)

	reader := bufio.NewReader(c)
	ch := make(chan *ScanResult)

	go func() {
		defer func() {
			close(ch)
			wg.Done()
		}()

		for {
			line, err := reader.ReadString('\n')
			if err == io.EOF {
				return
			}
			if err != nil {
				return
			}
			line = strings.TrimRight(line, " \t\r\n")
			ch <- parseResult(line)
		}
	}()

	return ch, &wg, nil
}

func parseResult(line string) *ScanResult {
	res := &ScanResult{Raw: line}

	matches := resultRegex.FindStringSubmatch(line)
	if len(matches) == 0 {
		res.Description = "Regex had no matches"
		res.Status = ResParseError
		return res
	}

	for i, name := range resultRegex.SubexpNames() {
		switch name {
		case "path":
			res.Path = matches[i]
		case "desc":
			res.Description = matches[i]
		case "virhash":
			res.Hash = matches[i]
		case "virsize":
			if n, err := strconv.Atoi(matches[i]); err == nil {
				res.Size = n
			}
		case "status":
			switch matches[i] {
			case ResOK, ResFound, ResError:
				res.Status = matches[i]
			default:
				res.Description = "Invalid status field: " + matches[i]
				res.Status = ResParseError
				return res
			}
		}
	}

	return res
}

func newCLAMDTcpConn(address string) (*CLAMDConn, error) {
	conn, err := net.DialTimeout("tcp", address, TCP_TIMEOUT)
	if err != nil {
		return nil, err
	}
	return &CLAMDConn{Conn: conn}, nil
}

func newCLAMDUnixConn(address string) (*CLAMDConn, error) {
	conn, err := net.Dial("unix", address)
	if err != nil {
		return nil, err
	}
	return &CLAMDConn{Conn: conn}, nil
}
