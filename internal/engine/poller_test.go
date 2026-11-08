// SPDX-License-Identifier: MIT

package engine

import (
	"testing"
	"time"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

func TestPollingLifecycle(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	events, _ := e.Subscribe(ctx, 0)
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	for name, cfg := range map[string]model.PollConfig{
		"nothing": {IntervalMS: 1000},
		"slow":    {Refs: []string{"x"}, IntervalMS: model.MaxPollIntervalMS + 1},
		"many":    {Refs: make([]string, model.MaxReadRefs+1)},
	} {
		if err := e.StartPolling(ctx, info.ID, cfg); service.CodeOf(err) == "" {
			t.Errorf("%s accepted", name)
		}
	}
	if err := e.StartPolling(ctx, "nope", model.PollConfig{Refs: []string{"x"}}); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown session = %v", err)
	}

	// A bad reference fails in every sample; the failures are counted.
	refs := []string{simied.Domain + "/MMXU1.Hz.mag.f[MX]", "nope["}
	if err := e.StartPolling(ctx, info.ID, model.PollConfig{Refs: refs, IntervalMS: model.MinPollIntervalMS}); err != nil {
		t.Fatal(err)
	}
	ev := waitEvent(t, events, func(ev model.Event) bool { return ev.Kind == model.EventSample })
	if ev.Sample.Seq != 1 || ev.Sample.Values[0].Value == nil || ev.Sample.Values[1].Error == nil {
		t.Errorf("first sample = %+v", ev.Sample)
	}
	// Replacing the poller restarts the count.
	if err := e.StartPolling(ctx, info.ID, model.PollConfig{Refs: refs[:1], IntervalMS: model.MinPollIntervalMS}); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, events, func(ev model.Event) bool { return ev.Kind == model.EventSample && ev.Sample.Seq == 1 && len(ev.Sample.Values) == 1 })
	list, _ := e.Sessions(ctx)
	if p := list[0].Polling; p == nil || p.Samples < 1 || p.IntervalMS != model.MinPollIntervalMS {
		t.Errorf("state = %+v", p)
	}

	// The IED goes away: polling stops with the session.
	ied.Close()
	waitEvent(t, events, func(ev model.Event) bool { return ev.Kind == model.EventSession && ev.Session.State == model.SessionLost })
	deadline := time.Now().Add(5 * time.Second)
	for {
		list, _ = e.Sessions(ctx)
		if list[0].Polling == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if list[0].Polling != nil {
		t.Errorf("still polling a lost session: %+v", list[0].Polling)
	}
	if err := e.StartPolling(ctx, info.ID, model.PollConfig{Refs: refs}); service.CodeOf(err) != service.CodeSessionLost {
		t.Errorf("poll lost session = %v", err)
	}
}

// A cycle slower than the interval skips ticks and counts them.
func TestPollingOverruns(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	s, _ := e.session(info.ID)
	// Hold the session for a while: the poller's cycle waits behind it.
	s.mu.Lock()
	if err := e.StartPolling(ctx, info.ID, model.PollConfig{Refs: []string{simied.Domain + "/MMXU1.Hz.mag.f[MX]"}, IntervalMS: model.MinPollIntervalMS}); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	time.Sleep(3 * model.MinPollIntervalMS * time.Millisecond)
	s.mu.Unlock()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		list, _ := e.Sessions(ctx)
		if p := list[0].Polling; p != nil && p.Overruns > 0 {
			_ = e.StopPolling(ctx, info.ID)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no overrun counted")
}
