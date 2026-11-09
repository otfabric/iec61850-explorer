// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"net"
	"net/netip"
	"syscall"

	"github.com/otfabric/iec61850-explorer/internal/proxy"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// AddrRefusedError is the error of a connection that Options.AllowAddr
// refused when it was opened. classify names it by its type.
type AddrRefusedError struct {
	Addr netip.Addr
	Err  error
}

func (e *AddrRefusedError) Error() string { return e.Err.Error() }

func (e *AddrRefusedError) Unwrap() error { return e.Err }

// allowTarget runs the checks of Options.AllowTarget before connecting: the
// IED's host and, for a proxy the operator did not choose, the proxy's host.
// A proxy that resolves host names itself would bypass the check of the
// resolved address, so the IED must then be given as an IP address.
func (e *Engine) allowTarget(ctx context.Context, host string, via proxyChoice) error {
	if e.opts.AllowTarget == nil {
		return nil
	}
	if via.cfg != nil && via.cfg.ResolvesRemotely() {
		if _, err := netip.ParseAddr(host); err != nil {
			return service.Errorf(service.CodeNotAllowed,
				"the proxy %s resolves host names itself, so the daemon's target allow-list cannot check %q: give the IED's IP address", via.cfg.Redacted(), host)
		}
	}
	if err := e.opts.AllowTarget(ctx, host); err != nil {
		return service.Errorf(service.CodeNotAllowed, "%v", err)
	}
	if via.cfg != nil && !via.trusted {
		if err := e.opts.AllowTarget(ctx, via.cfg.Host); err != nil {
			return service.Errorf(service.CodeNotAllowed, "proxy %s: %v", via.cfg.Redacted(), err)
		}
	}
	return nil
}

// dialer returns the go-iec61850 Dialer of a connection through via: nil
// (net.Dialer) for a direct connection without AllowAddr.
func (e *Engine) dialer(via proxyChoice) proxy.DialFunc {
	check := e.opts.AllowAddr
	var checked proxy.DialFunc
	if check != nil {
		d := net.Dialer{Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return &AddrRefusedError{Err: err}
			}
			addr := ap.Addr().Unmap().WithZone("")
			if err := check(addr); err != nil {
				return &AddrRefusedError{Addr: addr, Err: err}
			}
			return nil
		}}
		checked = d.DialContext
	}
	if via.cfg == nil {
		return checked
	}
	opts := proxy.Options{}
	if check != nil {
		opts.CheckAddr = func(a netip.Addr) error {
			if err := check(a); err != nil {
				return &AddrRefusedError{Addr: a, Err: err}
			}
			return nil
		}
		if !via.trusted {
			opts.Base = checked
		}
	}
	return proxy.Dialer(*via.cfg, opts)
}
