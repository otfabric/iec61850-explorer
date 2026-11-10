// SPDX-License-Identifier: MIT

package scenario

import (
	"context"
	"fmt"
	"time"
)

// All are the scenarios in file order.
func All() []Scenario { return []Scenario{S01, S02, S03, S04, S05, S06, S07, S08} }

// Record runs a scenario and returns its pcapng file.
func Record(s Scenario, names ...string) ([]byte, error) {
	e, err := NewEnv(s.Start, s.Seed, names...)
	if err != nil {
		return nil, err
	}
	defer e.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.Run(ctx, e); err != nil {
		return nil, fmt.Errorf("%s: %w", s.File, err)
	}
	return e.Cap.PCAPNG(s.Comment), nil
}
