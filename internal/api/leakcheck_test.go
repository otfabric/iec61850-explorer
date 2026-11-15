// SPDX-License-Identifier: MIT

package api

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The goroutine leak check (TEST-006): TestMain fails the package when a
// goroutine that runs code of this repository or of the OT Fabric libraries
// is left after every test has ended and cleaned up: the engines behind the
// HTTP API, their sessions and the simulated IEDs. It compares goroutines by
// ID and stack, not by count (the same check as internal/engine's).

func TestMain(m *testing.M) {
	before := goroutineStacks()
	code := m.Run()
	if left := leftGoroutines("github.com/otfabric/", before); len(left) > 0 && code == 0 {
		fmt.Fprintf(os.Stderr, "%d goroutines left after the tests:\n\n%s\n", len(left), strings.Join(left, "\n\n"))
		code = 1
	}
	os.Exit(code)
}

// leftGoroutines returns the stacks of the goroutines that contain frame
// and are not in before. A goroutine leaves the stack shortly after it
// signalled its end, so it waits for the condition, bounded, as goleak does.
func leftGoroutines(frame string, before map[string]string) []string {
	deadline := time.Now().Add(5 * time.Second)
	for {
		var left []string
		for id, stack := range goroutineStacks() {
			if _, old := before[id]; old || !strings.Contains(stack, frame) || strings.Contains(stack, "leftGoroutines") {
				continue
			}
			left = append(left, stack)
		}
		if len(left) == 0 || time.Now().After(deadline) {
			return left
		}
		runtime.Gosched()
		time.Sleep(5 * time.Millisecond) // polling a condition with a deadline, not synchronising
	}
}

// goroutineStacks maps the ID of every goroutine to its stack.
func goroutineStacks() map[string]string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	out := map[string]string{}
	for _, g := range strings.Split(string(buf), "\n\n") {
		head, _, _ := strings.Cut(g, "\n")
		if f := strings.Fields(head); len(f) >= 2 && f[0] == "goroutine" {
			out[f[1]] = g
		}
	}
	return out
}
