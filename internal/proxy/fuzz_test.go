// SPDX-License-Identifier: MIT

package proxy

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"
)

// A SOCKS5 reply is accepted only when it is complete and well formed; any
// failure is a *Error with a stage.
func FuzzSOCKSReply(f *testing.F) {
	f.Add([]byte("\x05\x00\x00\x01\x7f\x00\x00\x01\x00\x50"))
	f.Add([]byte("\x05\x05\x00\x03\x02ab\x00\x00"))
	f.Add([]byte("\x05\x00\x00\x04" + strings.Repeat("\x00", 18)))
	f.Add([]byte("\x05\x00\x00\x03"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		r := bytes.NewReader(data)
		err := readReply(r)
		if err == nil {
			if len(data) < 7 || data[0] != 5 || data[1] != 0 || data[2] != 0 || (data[3] != 1 && data[3] != 3 && data[3] != 4) {
				t.Fatalf("accepted %x", data)
			}
			return
		}
		var pe *Error
		if !errors.As(err, &pe) || pe.Stage == "" {
			t.Fatalf("err = %v", err)
		}
	})
}

// The answer to CONNECT is read within the header bound; a status is
// returned only with the complete header.
func FuzzHTTPResponse(f *testing.F) {
	f.Add([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	f.Add([]byte("HTTP/1.0 407 Auth\r\nProxy-Authenticate: Basic\r\n\r\nrest"))
	f.Add([]byte("HTTP/1.1 200 OK\n\n"))
	f.Add([]byte("HTTP/1.1 2000\r\n\r\n"))
	f.Add([]byte("garbage"))
	f.Fuzz(func(t *testing.T, data []byte) {
		br := bufio.NewReaderSize(bytes.NewReader(data), 4096)
		status, err := readResponseHeader(br)
		if err != nil {
			var pe *Error
			if !errors.As(err, &pe) || pe.Stage != StageHandshake {
				t.Fatalf("err = %v", err)
			}
			return
		}
		read := len(data) - remaining(br)
		if status < 100 || status > 999 || read > maxResponseHeader {
			t.Fatalf("status %d after %d bytes", status, read)
		}
	})
}

func remaining(br *bufio.Reader) int {
	n := 0
	for {
		if _, err := br.ReadByte(); err != nil {
			return n
		}
		n++
	}
}

// The redacted form of a parsed URL parses back to the same proxy, with the
// password replaced.
func FuzzParse(f *testing.F) {
	f.Add("socks5://user:pw@host:1080")
	f.Add("http://user:pw@[::1]:3128")
	f.Add("socks5h://host")
	f.Add("ftp://x")
	f.Fuzz(func(t *testing.T, raw string) {
		c, err := Parse(raw)
		if err != nil {
			return
		}
		again, err := Parse(c.Redacted())
		if err != nil || again.Addr() != c.Addr() || again.Scheme != c.Scheme || again.user != c.user {
			t.Fatalf("redacted %q does not parse back: %v", c.Redacted(), err)
		}
		if again.hasPass != c.hasPass || (c.hasPass && again.pass != redactedPassword) {
			t.Fatalf("redacted %q holds a password", c.Redacted())
		}
	})
}
