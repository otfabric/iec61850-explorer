// SPDX-License-Identifier: MIT

package ied

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/demo/internal/site"
)

// simulate runs the process of a live IED: every 2 s the load and the
// frequency drift; every 45 s the transformer's voltage controller moves
// the tap changer by one step while it is in automatic mode.
func (i *IED) simulate(ctx context.Context) {
	seed := uint64(0)
	for _, c := range i.Site.Name {
		seed = seed*31 + uint64(c)
	}
	rng := rand.New(rand.NewPCG(seed, 61850))
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	scale, df, n := 1.0, 0.0, 0
	tap := int64(9)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		n++
		now := i.Now().Truncate(time.Millisecond)
		scale = min(1.15, max(0.85, scale+(rng.Float64()-0.5)*0.04))
		df = min(0.04, max(-0.04, df+(rng.Float64()-0.5)*0.01))
		i.mu.Lock()
		switch i.Site.Name {
		case site.F1Name, site.F2Name:
			i.feederMeas(now, scale, df, false)
		case site.T1Name:
			i.transformerMeas(now, scale)
			i.Meas("CTRL", "YLTC1", "MotDrvTmp", now, 31.4+float64(n%30)*0.05)
			if auto, _ := i.Get("CTRL", "ATCC1$ST$Auto$stVal").Bool(); auto && n%22 == 0 {
				if rng.IntN(2) == 0 && tap < 17 || tap <= 1 {
					tap++
				} else {
					tap--
				}
				i.tap(now, tap)
				cnt, _ := i.Get("CTRL", "YLTC1$ST$OpCnt$stVal").Int64()
				i.Status("CTRL", "YLTC1", "OpCnt", now, "stVal", mms.NewInteger(cnt+1))
			}
		}
		i.mu.Unlock()
	}
}
