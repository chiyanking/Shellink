package sessions

import (
	"testing"
	"time"
)

func TestWaitForStableAfterCommand(t *testing.T) {
	for _, test := range []struct {
		name       string
		output     string
		acceptIdle bool
		close      bool
		state      State
		timedOut   bool
	}{
		{name: "old prompt", state: WaitingInput, timedOut: true},
		{name: "fast command", output: "result\nshell$ ", state: WaitingInput},
		{name: "silent command", output: "shell$ ", state: WaitingInput},
		{name: "idle output", output: "result\n", state: Idle, timedOut: true},
		{name: "accepted idle output", output: "result\n", acceptIdle: true, state: Idle},
		{name: "disconnected", close: true, state: Disconnected},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, err := NewSession(SessionOptions{ID: "test", SilenceMS: 3_600_000}, NewBus())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { session.Close("test complete") })
			session.handleOutput("shell$ ")
			session.evaluateSilence()
			startSeq := session.LastSequence()
			// Recording input alone must not make the old prompt count as a
			// completed command. Real command output can settle before we wait.
			session.mu.Lock()
			session.recordLocked("input", "command\n", "command\n")
			session.mu.Unlock()
			if test.output != "" {
				session.handleOutput(test.output)
				session.evaluateSilence()
			}
			if test.close {
				session.Close("connection lost")
			}
			state, timedOut := session.WaitForStableAfter(startSeq, 20*time.Millisecond, test.acceptIdle)
			if state != test.state || timedOut != test.timedOut {
				t.Fatalf("WaitForStableAfter() = (%s, %t), want (%s, %t)", state, timedOut, test.state, test.timedOut)
			}
		})
	}
}
