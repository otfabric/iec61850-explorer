// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/model"
)

func TestError(t *testing.T) {
	e := Errorf(CodeNotFound, "no session %q", "abc")
	if e.Error() != `no session "abc"` || CodeOf(e) != CodeNotFound || e.Unwrap() != nil {
		t.Errorf("Errorf = %+v", e)
	}
	w := &Error{Code: CodeConnectFailed, Message: "cannot connect", Err: io.ErrUnexpectedEOF,
		Detail: &model.OpError{Code: model.ErrCodeConnect, Message: "refused", Stage: "tcp"}}
	if !errors.Is(w, io.ErrUnexpectedEOF) {
		t.Error("Unwrap")
	}
	if CodeOf(nil) != "" || CodeOf(io.EOF) != CodeInternal || CodeOf(errors.Join(io.EOF, w)) != CodeConnectFailed {
		t.Error("CodeOf")
	}

	// The wire form carries code, message and detail, not the wrapped error.
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"code":"connect_failed","message":"cannot connect","detail":{"code":"connect_failed","message":"refused","stage":"tcp"}}`
	if string(b) != want {
		t.Errorf("JSON = %s", b)
	}
}

func TestWrap(t *testing.T) {
	w := Wrap(CodeInternal, io.EOF, "save %s", "x")
	if w.Error() != "save x: EOF" || !errors.Is(w, io.EOF) || w.Code != CodeInternal {
		t.Errorf("Wrap = %+v", w)
	}
}
