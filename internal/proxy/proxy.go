// SPDX-License-Identifier: MIT

// Package proxy opens TCP connections through a SOCKS5 proxy (RFC 1928, with
// the username/password authentication of RFC 1929) or an HTTP proxy's
// CONNECT method (RFC 9110, section 9.3.6). The engine passes the dial
// function it returns to go-iec61850 (DialOptions.Dialer).
//
// A Config never prints its password: String, GoString and Redacted replace
// it, and errors carry the redacted form only.
package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// Scheme is the proxy protocol.
type Scheme string

// Schemes.
const (
	// SOCKS5 resolves the destination's host name locally and sends the
	// address.
	SOCKS5 Scheme = "socks5"
	// SOCKS5H sends the host name; the proxy resolves it.
	SOCKS5H Scheme = "socks5h"
	// HTTP uses the CONNECT method; the proxy resolves a host name.
	HTTP Scheme = "http"
)

// Direct is the value of a proxy setting that asks for no proxy.
const Direct = "direct"

// Default ports.
const (
	DefaultSOCKSPort = 1080
	DefaultHTTPPort  = 8080
)

// maxCredential bounds a user name or password: RFC 1929 carries each in at
// most 255 octets; the same bound applies to HTTP.
const maxCredential = 255

// redactedPassword replaces the password in Redacted.
const redactedPassword = "xxxxx"

// Config is a parsed proxy URL.
type Config struct {
	Scheme Scheme
	// Host is the proxy's host name or address, Port its port.
	Host string
	Port int
	user string
	pass string
	// hasPass records a password given, even an empty one.
	hasPass bool
}

// Parse parses socks5://[user[:pass]@]host[:port], socks5h://... or
// http://[user[:pass]@]host[:port]. The SOCKS port defaults to 1080, the HTTP
// port to 8080. A path, query or fragment is refused.
func Parse(raw string) (Config, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Config{}, errors.New("the proxy URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		// url.Error repeats the URL, password included.
		return Config{}, errors.New("the proxy is not a URL (socks5://, socks5h:// or http://host:port)")
	}
	var c Config
	switch strings.ToLower(u.Scheme) {
	case string(SOCKS5):
		c.Scheme, c.Port = SOCKS5, DefaultSOCKSPort
	case string(SOCKS5H):
		c.Scheme, c.Port = SOCKS5H, DefaultSOCKSPort
	case string(HTTP):
		c.Scheme, c.Port = HTTP, DefaultHTTPPort
	default:
		return Config{}, fmt.Errorf("proxy scheme %q is not supported: use socks5, socks5h or http", u.Scheme)
	}
	if u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return Config{}, errors.New("a proxy URL has no path, query or fragment")
	}
	c.Host = u.Hostname()
	if c.Host == "" {
		return Config{}, errors.New("the proxy URL has no host")
	}
	if strings.ContainsAny(c.Host, "/@?# %[]") || len(c.Host) > 253 {
		return Config{}, fmt.Errorf("%q is not a proxy host name or address", c.Host)
	}
	if _, err := netip.ParseAddr(c.Host); err != nil && strings.Contains(c.Host, ":") {
		return Config{}, fmt.Errorf("%q is not a proxy host name or address", c.Host)
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return Config{}, fmt.Errorf("proxy port %q is out of range", p)
		}
		c.Port = n
	} else if strings.HasSuffix(u.Host, ":") {
		return Config{}, errors.New("the proxy URL has an empty port")
	}
	if u.User != nil {
		c.user = u.User.Username()
		c.pass, c.hasPass = u.User.Password()
		if c.user == "" {
			return Config{}, errors.New("a proxy password needs a user name")
		}
		if len(c.user) > maxCredential || len(c.pass) > maxCredential {
			return Config{}, fmt.Errorf("a proxy user name or password has at most %d bytes", maxCredential)
		}
		if strings.ContainsFunc(c.user+c.pass, isControl) {
			return Config{}, errors.New("the proxy credentials contain control characters")
		}
		if c.Scheme == HTTP && strings.Contains(c.user, ":") {
			return Config{}, errors.New("an HTTP proxy user name cannot contain ':'")
		}
	}
	return c, nil
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }

// Username is the user name, "" without authentication.
func (c Config) Username() string { return c.user }

// HasPassword reports whether the URL gave a password.
func (c Config) HasPassword() bool { return c.hasPass }

// Addr is the proxy's host:port.
func (c Config) Addr() string { return net.JoinHostPort(c.Host, strconv.Itoa(c.Port)) }

// ResolvesRemotely reports whether the proxy, not this program, resolves the
// destination's host name (socks5h and HTTP CONNECT).
func (c Config) ResolvesRemotely() bool { return c.Scheme != SOCKS5 }

// Redacted is the URL with the password replaced; it never contains the
// password.
func (c Config) Redacted() string {
	if c.Scheme == "" {
		return ""
	}
	u := url.URL{Scheme: string(c.Scheme), Host: c.Addr()}
	switch {
	case c.hasPass:
		u.User = url.UserPassword(c.user, redactedPassword)
	case c.user != "":
		u.User = url.User(c.user)
	}
	return u.String()
}

// String is Redacted.
func (c Config) String() string { return c.Redacted() }

// GoString is Redacted, so that %#v does not print the password either.
func (c Config) GoString() string { return "proxy.Config(" + c.Redacted() + ")" }

// Stage says where a connection through the proxy failed.
type Stage string

// Stages of Error.
const (
	// StageConnect: no TCP connection to the proxy.
	StageConnect Stage = "connect"
	// StageResolve: the destination's host name could not be resolved
	// locally (socks5).
	StageResolve Stage = "resolve"
	// StageNotAllowed: the address the destination resolved to was
	// refused by CheckAddr.
	StageNotAllowed Stage = "not-allowed"
	// StageHandshake: the proxy's answer was malformed, too long or cut
	// off, or no answer came in time.
	StageHandshake Stage = "handshake"
	// StageAuth: the proxy wants authentication the configuration does not
	// give, or refused the credentials.
	StageAuth Stage = "auth"
	// StageRefused: the proxy refused to connect to the destination; Reply
	// (SOCKS) or Status (HTTP) says why.
	StageRefused Stage = "refused"
)

// Error is a failure to connect through a proxy.
type Error struct {
	// Proxy is the redacted proxy URL.
	Proxy string
	Stage Stage
	// Reply is the SOCKS5 reply code of a refusal (StageRefused), else 0.
	Reply int
	// Status is the HTTP status of a refused CONNECT (StageRefused or
	// StageAuth), else 0.
	Status int
	Err    error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("proxy %s: %s", e.Proxy, e.Stage)
	switch {
	case e.Reply != 0:
		msg += fmt.Sprintf(": SOCKS reply %d (%s)", e.Reply, ReplyText(e.Reply))
	case e.Status != 0:
		msg += fmt.Sprintf(": HTTP status %d", e.Status)
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// ReplyText names a SOCKS5 reply code (RFC 1928, section 6).
func ReplyText(code int) string {
	switch code {
	case 1:
		return "general SOCKS server failure"
	case 2:
		return "connection not allowed by ruleset"
	case 3:
		return "network unreachable"
	case 4:
		return "host unreachable"
	case 5:
		return "connection refused"
	case 6:
		return "TTL expired"
	case 7:
		return "command not supported"
	case 8:
		return "address type not supported"
	}
	return "unassigned"
}

// DialFunc opens a connection; net.Dialer.DialContext is one.
type DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error)

// Options configures Dialer.
type Options struct {
	// Base opens the TCP connection to the proxy. Nil uses a zero
	// net.Dialer.
	Base DialFunc
	// Resolver resolves the destination's host name for socks5. Nil uses
	// net.DefaultResolver.
	Resolver func(ctx context.Context, host string) ([]netip.Addr, error)
	// CheckAddr, when set, is asked for the destination address socks5
	// resolved locally, or that a destination gave as an address, before
	// it is sent to the proxy. An error refuses it (StageNotAllowed).
	CheckAddr func(netip.Addr) error
}

// Dialer returns a dial function that connects to addr ("host:port") through
// the proxy. The context bounds the connection to the proxy and the
// handshake; the returned connection carries no deadline.
func Dialer(cfg Config, opts Options) DialFunc {
	if opts.Base == nil {
		var d net.Dialer
		opts.Base = d.DialContext
	}
	if opts.Resolver == nil {
		opts.Resolver = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dial(ctx, cfg, opts, network, addr)
	}
}

func dial(ctx context.Context, cfg Config, opts Options, network, addr string) (net.Conn, error) {
	fail := func(stage Stage, err error) error { return &Error{Proxy: cfg.Redacted(), Stage: stage, Err: err} }
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, fail(StageConnect, fmt.Errorf("network %q is not supported through a proxy", network))
	}
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fail(StageConnect, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, fail(StageConnect, fmt.Errorf("port %q is out of range", portText))
	}
	if len(host) > 255 {
		return nil, fail(StageConnect, errors.New("the destination host name is longer than 255 bytes"))
	}
	dst := destination{host: host, port: port}
	if a, err := netip.ParseAddr(host); err == nil {
		dst.addr = a.Unmap().WithZone("")
	} else if cfg.Scheme == SOCKS5 {
		addrs, err := opts.Resolver(ctx, host)
		if err != nil || len(addrs) == 0 {
			if err == nil {
				err = errors.New("no address")
			}
			return nil, fail(StageResolve, fmt.Errorf("%s: %w", host, err))
		}
		dst.addr = addrs[0].Unmap().WithZone("")
	}
	if dst.addr.IsValid() && opts.CheckAddr != nil {
		if err := opts.CheckAddr(dst.addr); err != nil {
			return nil, fail(StageNotAllowed, err)
		}
	}

	conn, err := opts.Base(ctx, "tcp", cfg.Addr())
	if err != nil {
		return nil, fail(StageConnect, err)
	}
	// The context bounds the handshake: its deadline on the connection, and
	// a cancellation closes it.
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	var out net.Conn
	if cfg.Scheme == HTTP {
		out, err = connectHTTP(conn, cfg, dst)
	} else {
		err = connectSOCKS(conn, cfg, dst)
		out = conn
	}
	if !stop() {
		// The context ended during the handshake and closed the connection.
		_ = conn.Close()
		var pe *Error
		if errors.As(err, &pe) {
			pe.Proxy = cfg.Redacted()
			if pe.Err != nil {
				pe.Err = fmt.Errorf("%w (%w)", ctx.Err(), pe.Err)
			} else {
				pe.Err = ctx.Err()
			}
			return nil, pe
		}
		return nil, fail(StageHandshake, ctx.Err())
	}
	if err != nil {
		_ = conn.Close()
		var pe *Error
		if errors.As(err, &pe) {
			pe.Proxy = cfg.Redacted()
			return nil, pe
		}
		return nil, fail(StageHandshake, err)
	}
	_ = conn.SetDeadline(noDeadline)
	return out, nil
}

// destination is where the proxy connects to. addr is set for an address
// (given, or resolved locally for socks5); host is the name otherwise.
type destination struct {
	host string
	addr netip.Addr
	port int
}

func (d destination) hostPort() string {
	if d.addr.IsValid() {
		return net.JoinHostPort(d.addr.String(), strconv.Itoa(d.port))
	}
	return net.JoinHostPort(d.host, strconv.Itoa(d.port))
}
