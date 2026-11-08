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
		refErr  *iec61850.ReferenceError
	)
	switch {
	case errors.Is(err, mms.ErrServiceNotAllowed):
		e.Code, e.Stage = model.ErrCodeNotAllowed, stageRequest
		e.Hint = "The Explorer is in Observe mode and does not send state-changing MMS services. Nothing was sent."
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
