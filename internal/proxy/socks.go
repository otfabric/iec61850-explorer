// SPDX-License-Identifier: MIT

package proxy

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// noDeadline clears a connection's deadline.
var noDeadline time.Time

// SOCKS5 constants (RFC 1928, RFC 1929).
const (
	socksVersion   = 5
	authNone       = 0x00
	authUserPass   = 0x02
	authNoAccept   = 0xff
	userPassVer    = 1
	cmdConnect     = 1
	atypIPv4       = 1
	atypDomain     = 3
	atypIPv6       = 4
	replySucceeded = 0
)

// connectSOCKS runs the SOCKS5 handshake for a CONNECT to dst.
func connectSOCKS(conn net.Conn, cfg Config, dst destination) error {
	methods := []byte{authNone}
	if cfg.user != "" {
		methods = []byte{authUserPass}
	}
	hello := append([]byte{socksVersion, byte(len(methods))}, methods...)
	if _, err := conn.Write(hello); err != nil {
		return &Error{Stage: StageHandshake, Err: err}
	}
	var sel [2]byte
	if _, err := io.ReadFull(conn, sel[:]); err != nil {
		return &Error{Stage: StageHandshake, Err: err}
	}
	method, err := parseMethodSelection(sel[:])
	if err != nil {
		return err
	}
	switch method {
	case authNone:
		if cfg.user != "" {
			// We offered only username/password.
			return &Error{Stage: StageHandshake, Err: errors.New("the proxy chose no authentication, which was not offered")}
		}
	case authUserPass:
		if cfg.user == "" {
			return &Error{Stage: StageHandshake, Err: errors.New("the proxy chose username/password authentication, which was not offered")}
		}
		if err := authenticate(conn, cfg); err != nil {
			return err
		}
	case authNoAccept:
		if cfg.user == "" {
			return &Error{Stage: StageAuth, Err: errors.New("the proxy requires authentication (give user:password in the proxy URL)")}
		}
		return &Error{Stage: StageAuth, Err: errors.New("the proxy accepts no offered authentication method")}
	default:
		return &Error{Stage: StageHandshake, Err: fmt.Errorf("the proxy chose method %#02x, which was not offered", method)}
	}

	req, err := connectRequest(dst)
	if err != nil {
		return &Error{Stage: StageHandshake, Err: err}
	}
	if _, err := conn.Write(req); err != nil {
		return &Error{Stage: StageHandshake, Err: err}
	}
	return readReply(conn)
}

// parseMethodSelection checks the server's method selection message.
func parseMethodSelection(b []byte) (byte, error) {
	if b[0] != socksVersion {
		return 0, &Error{Stage: StageHandshake, Err: fmt.Errorf("the proxy answered with version %d, not SOCKS5", b[0])}
	}
	return b[1], nil
}

// authenticate runs RFC 1929 username/password authentication.
func authenticate(conn net.Conn, cfg Config) error {
	msg := make([]byte, 0, 3+len(cfg.user)+len(cfg.pass))
	msg = append(msg, userPassVer, byte(len(cfg.user)))
	msg = append(msg, cfg.user...)
	msg = append(msg, byte(len(cfg.pass)))
	msg = append(msg, cfg.pass...)
	if _, err := conn.Write(msg); err != nil {
		return &Error{Stage: StageAuth, Err: err}
	}
	var resp [2]byte
	if _, err := io.ReadFull(conn, resp[:]); err != nil {
		return &Error{Stage: StageAuth, Err: err}
	}
	return parseAuthReply(resp[:])
}

// parseAuthReply checks an RFC 1929 status message.
func parseAuthReply(b []byte) error {
	if b[0] != userPassVer {
		return &Error{Stage: StageHandshake, Err: fmt.Errorf("authentication reply version %d, want 1", b[0])}
	}
	if b[1] != 0 {
		return &Error{Stage: StageAuth, Err: fmt.Errorf("the proxy refused the user name or password (status %d)", b[1])}
	}
	return nil
}

// connectRequest encodes a CONNECT request.
func connectRequest(dst destination) ([]byte, error) {
	req := []byte{socksVersion, cmdConnect, 0}
	switch {
	case dst.addr.Is4():
		a := dst.addr.As4()
		req = append(append(req, atypIPv4), a[:]...)
	case dst.addr.Is6():
		a := dst.addr.As16()
		req = append(append(req, atypIPv6), a[:]...)
	default:
		if dst.host == "" || len(dst.host) > 255 {
			return nil, errors.New("the destination host name has 1 to 255 bytes")
		}
		req = append(append(req, atypDomain, byte(len(dst.host))), dst.host...)
	}
	return binary.BigEndian.AppendUint16(req, uint16(dst.port)), nil
}

// readReply reads the reply to CONNECT, bound address included, so that no
// byte of it is left for the caller.
func readReply(r io.Reader) error {
	var head [4]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return &Error{Stage: StageHandshake, Err: err}
	}
	n, err := replyAddrLen(head[:], r)
	if err != nil {
		return err
	}
	if _, err := io.CopyN(io.Discard, r, int64(n)+2); err != nil {
		return &Error{Stage: StageHandshake, Err: err}
	}
	if head[1] != replySucceeded {
		return &Error{Stage: StageRefused, Reply: int(head[1])}
	}
	return nil
}

// replyAddrLen checks a reply's first four octets and returns the length of
// its bound address; for a domain name it reads the length octet from r.
func replyAddrLen(head []byte, r io.Reader) (int, error) {
	if head[0] != socksVersion {
		return 0, &Error{Stage: StageHandshake, Err: fmt.Errorf("the proxy answered with version %d, not SOCKS5", head[0])}
	}
	if head[2] != 0 {
		return 0, &Error{Stage: StageHandshake, Err: fmt.Errorf("reserved octet %#02x in the reply", head[2])}
	}
	switch head[3] {
	case atypIPv4:
		return 4, nil
	case atypIPv6:
		return 16, nil
	case atypDomain:
		var l [1]byte
		if _, err := io.ReadFull(r, l[:]); err != nil {
			return 0, &Error{Stage: StageHandshake, Err: err}
		}
		return int(l[0]), nil
	}
	return 0, &Error{Stage: StageHandshake, Err: fmt.Errorf("address type %d in the reply", head[3])}
}
