// SPDX-License-Identifier: MIT

package proxy

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

// maxResponseHeader bounds the proxy's answer to CONNECT, status line and
// header fields together.
const maxResponseHeader = 8 << 10

// connectHTTP sends CONNECT and reads the answer. Bytes the proxy sent after
// the header (the destination speaking first) stay readable from the
// returned connection.
func connectHTTP(conn net.Conn, cfg Config, dst destination) (net.Conn, error) {
	target := dst.hostPort()
	var b strings.Builder
	fmt.Fprintf(&b, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target)
	if cfg.user != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(cfg.user + ":" + cfg.pass))
		b.WriteString("Proxy-Authorization: Basic " + cred + "\r\n")
	}
	b.WriteString("\r\n")
	if _, err := io.WriteString(conn, b.String()); err != nil {
		return nil, &Error{Stage: StageHandshake, Err: err}
	}
	br := bufio.NewReaderSize(conn, 4096)
	status, err := readResponseHeader(br)
	if err != nil {
		return nil, err
	}
	switch {
	case status >= 200 && status <= 299:
	case status == 407:
		return nil, &Error{Stage: StageAuth, Status: status, Err: errors.New("the proxy requires authentication or refused the credentials")}
	default:
		return nil, &Error{Stage: StageRefused, Status: status}
	}
	if br.Buffered() == 0 {
		return conn, nil
	}
	rest, _ := br.Peek(br.Buffered())
	return &bufferedConn{Conn: conn, buf: bytes.Clone(rest)}, nil
}

// readResponseHeader reads a response's status line and header fields, at
// most maxResponseHeader octets, and returns the status code.
func readResponseHeader(br *bufio.Reader) (int, error) {
	total := 0
	status := 0
	for first := true; ; first = false {
		line, err := readLine(br, maxResponseHeader-total)
		if err != nil {
			return 0, err
		}
		total += len(line)
		trimmed := strings.TrimRight(line, "\r\n")
		if first {
			if status, err = parseStatusLine(trimmed); err != nil {
				return 0, err
			}
			continue
		}
		if trimmed == "" {
			return status, nil
		}
	}
}

// readLine reads one line ending in LF of at most limit octets.
func readLine(br *bufio.Reader, limit int) (string, error) {
	var line []byte
	for {
		if limit <= 0 {
			return "", &Error{Stage: StageHandshake, Err: fmt.Errorf("the proxy's response header is longer than %d bytes", maxResponseHeader)}
		}
		c, err := br.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return "", &Error{Stage: StageHandshake, Err: err}
		}
		line = append(line, c)
		limit--
		if c == '\n' {
			return string(line), nil
		}
	}
}

// parseStatusLine parses "HTTP/1.x NNN reason".
func parseStatusLine(line string) (int, error) {
	proto, rest, ok := strings.Cut(line, " ")
	if !ok || (proto != "HTTP/1.1" && proto != "HTTP/1.0") {
		return 0, &Error{Stage: StageHandshake, Err: fmt.Errorf("the proxy's answer is not an HTTP/1.x status line: %q", truncate(line))}
	}
	code, _, _ := strings.Cut(rest, " ")
	n, err := strconv.Atoi(code)
	if err != nil || len(code) != 3 || n < 100 {
		return 0, &Error{Stage: StageHandshake, Err: fmt.Errorf("status code %q", truncate(code))}
	}
	return n, nil
}

func truncate(s string) string {
	const max = 64
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// bufferedConn returns bytes read past the CONNECT response first.
type bufferedConn struct {
	net.Conn
	buf []byte
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	if len(c.buf) > 0 {
		n := copy(p, c.buf)
		c.buf = c.buf[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}
