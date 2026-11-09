// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/otfabric/go-cotp"
	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/proxy"
)

// Stages of OpError.Stage. A stage is named only when the error type says
// where the exchange stopped; nothing is inferred from a message text.
const (
	stageTCP          = "tcp"
	stageCOTP         = "cotp"
	stageSession      = "session"
	stagePresentation = "presentation"
	stageACSE         = "acse"
	stageInitiate     = "mms-initiate"
	stageRequest      = "request"
	stageResponse     = "response"
	stageProxy        = "proxy"
)

// classify turns an error of go-iec61850, go-mms or go-cotp into what the
// user sees. connecting says that err came from establishing the
// association, so that a failure can be placed in its layer.
func classify(err error, connecting bool) *model.OpError {
	if err == nil {
		return nil
	}
	e := &model.OpError{Code: model.ErrCodeOther, Message: err.Error()}

	var (
		netErr  *net.OpError
		rej     *cotp.RejectionError
		proto   *mms.ProtocolError
		svcErr  *mms.ServiceError
		rejErr  *mms.RejectError
		dataErr *mms.DataAccessError
		iecData *iec61850.DataAccessError
		refErr  *iec61850.ReferenceError
		pxErr   *proxy.Error
		refused *AddrRefusedError
	)
	// go-iec61850 reports a per-item read error with its own type and the
	// ISO 9506 data-access-error code.
	if errors.As(err, &iecData) && !errors.As(err, &dataErr) {
		dataErr = &mms.DataAccessError{Code: mms.DataAccessErrorCode(iecData.ErrorCode)}
		err = fmt.Errorf("%w: %w", err, dataErr)
		e.Message = fmt.Sprintf("%s: %s", iecData.Ref, dataErr.Code)
	}
	switch {
	case errors.Is(err, mms.ErrServiceNotAllowed):
		e.Code, e.Stage = model.ErrCodeNotAllowed, stageRequest
		e.Hint = "The Explorer is in Observe mode and does not send state-changing MMS services. Nothing was sent."
	case errors.As(err, &pxErr):
		classifyProxy(e, pxErr)
	case errors.As(err, &refused):
		e.Code, e.Stage = model.ErrCodeNotAllowed, stageTCP
		e.Message = fmt.Sprintf("the connection to %s was refused before it was opened: %v", refused.Addr, refused.Err)
		e.Hint = "The address is outside the daemon's allowed targets (--allow-target). A host name is checked again at the address it resolves to when connecting."
	case errors.As(err, &refErr):
		e.Code = model.ErrCodeInvalidRef
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, mms.ErrInvokeTimeout):
		e.Code = model.ErrCodeTimeout
		if !connecting {
			e.Stage = stageResponse
			e.Hint = "The request was sent and no answer came within the timeout. The association may still be usable."
		}
	case errors.As(err, &dataErr):
		e.Stage = stageResponse
		switch dataErr.Code {
		case mms.DataAccessErrorObjectNonExistent:
			e.Code = model.ErrCodeNotFound
			e.Hint = "The IED answered that the object does not exist. Check the reference and its functional constraint."
		case mms.DataAccessErrorObjectAccessDenied:
			e.Code = model.ErrCodeAccessDenied
		default:
			e.Code = model.ErrCodeServiceError
		}
	case errors.As(err, &svcErr):
		e.Code, e.Stage = model.ErrCodeServiceError, stageResponse
	case errors.As(err, &rejErr):
		e.Code, e.Stage = model.ErrCodeRejected, stageResponse
		e.Hint = "The IED rejected the request PDU (MMS Reject). It did not process it."
	case errors.As(err, &rej):
		e.Code, e.Stage = model.ErrCodeConnect, stageCOTP
		e.Message = fmt.Sprintf("the IED refused the transport connection (COTP disconnect request, reason %d)", rej.Reason)
		if rej.Reason == 3 {
			e.Hint = "Reason 3 is 'address unknown': the IED does not accept the T-selector the Explorer sent (0001). Check the OSI-TSEL in the IED's SCL."
		}
	case errors.Is(err, mms.ErrNegotiationFailed):
		e.Code, e.Stage = model.ErrCodeAssociation, stageInitiate
	case errors.Is(err, mms.ErrConnectionRejected):
		e.Code, e.Stage = model.ErrCodeAssociation, stageACSE
		e.Hint = "TCP and the transport connection stood; the IED rejected the association. Check the AP-title, AE-qualifier, selectors and any authentication the IED requires."
	case errors.As(err, &proto) && connecting:
		e.Code = model.ErrCodeAssociation
		switch proto.Phase {
		case mms.PhaseSession:
			e.Stage = stageSession
		case mms.PhasePresentation:
			e.Stage = stagePresentation
		case mms.PhaseACSE:
			e.Stage = stageACSE
		case mms.PhaseMMS:
			e.Stage = stageInitiate
		}
	case errors.Is(err, mms.ErrAssociationFailed):
		e.Code = model.ErrCodeAssociation
	case errors.As(err, &netErr) && netErr.Op == "dial":
		e.Code, e.Stage = model.ErrCodeConnect, stageTCP
		e.Hint = "No TCP connection to the IED. Check the address, the port (102 by default), routing and firewalls."
	case errors.Is(err, mms.ErrClosed), errors.Is(err, iec61850.ErrClosed):
		e.Code = model.ErrCodeConnectionLost
	case errors.Is(err, mms.ErrDecodeFailed):
		e.Code, e.Stage = model.ErrCodeDecode, stageResponse
	}
	return e
}

// classifyProxy places a failure to connect through a proxy: the IED was not
// reached, so no layer above TCP is involved.
func classifyProxy(e *model.OpError, pe *proxy.Error) {
	e.Code, e.Stage = model.ErrCodeConnect, stageProxy
	switch pe.Stage {
	case proxy.StageConnect:
		e.Hint = "No TCP connection to the proxy. Check the proxy's address and port and that it is running."
	case proxy.StageResolve:
		e.Hint = "The IED's host name could not be resolved here (socks5 resolves locally; socks5h lets the proxy resolve it)."
	case proxy.StageNotAllowed:
		e.Code = model.ErrCodeNotAllowed
		e.Hint = "The IED's address is outside the daemon's allowed targets (--allow-target)."
	case proxy.StageHandshake:
		e.Hint = "The proxy did not answer as a SOCKS5 or HTTP CONNECT proxy, or not in time. Check the proxy URL's scheme and port."
	case proxy.StageAuth:
		e.Code = model.ErrCodeAccessDenied
		e.Hint = "The proxy requires authentication or refused the user name and password of the proxy URL."
	case proxy.StageRefused:
		e.Hint = "The proxy was reached and refused to connect to the IED. Check the IED's address and port and the proxy's rules."
	}
	var refused *AddrRefusedError
	if errors.As(pe, &refused) {
		e.Code = model.ErrCodeNotAllowed
		e.Hint = "The address is outside the daemon's allowed targets (--allow-target): the proxy's when connecting to it, or the IED's that socks5 resolved."
	}
	if errors.Is(pe, context.DeadlineExceeded) {
		e.Code = model.ErrCodeTimeout
	}
}
