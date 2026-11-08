// SPDX-License-Identifier: MIT

package simied

import (
	"context"
	"testing"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"
)

func TestServesAndCounts(t *testing.T) {
	ied, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer ied.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := iec61850.Dial(ctx, ied.Addr, iec61850.DialOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := iec61850.ParseRef(Domain + "/MMXU1.Hz.mag.f[MX]")
	v, err := c.Read(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := v.Float32(); err != nil || f < 50.001 || f > 50.003 {
		t.Errorf("Hz = %v, %v", f, err)
	}
	_ = c.Close(ctx)
	if got := ied.Services()[mms.ServiceRead]; got != 1 {
		t.Errorf("Read count = %d", got)
	}
	if ied.Connections() != 1 {
		t.Errorf("connections = %d", ied.Connections())
	}
	ied.Close()
	ied.Close() // a second Close returns at once
}
