package legacy

import (
	"bytes"
	"errors"
	"strings"
	"sync"

	"github.com/canonical/tc"
)

type outputSuite struct{}

var _ = tc.InternalSuite(&outputSuite{})

// TestConcurrentWrites checks that stdout, stderr and hook-tool writers sharing a lock cannot corrupt their destination.
func (*outputSuite) TestConcurrentWrites(c *tc.C) {
	var output bytes.Buffer
	var mu sync.Mutex
	writers := []lockedWriter{{&mu, &output}, {&mu, &output}, {&mu, &output}}
	var workers sync.WaitGroup
	for _, writer := range writers {
		workers.Add(1)
		go func(writer lockedWriter) {
			defer workers.Done()
			for i := 0; i < 100; i++ {
				_, _ = writer.Write([]byte("complete-line\n"))
			}
		}(writer)
	}
	workers.Wait()

	c.Check(output.String(), tc.Equals, strings.Repeat("complete-line\n", 300))
}

// TestWriteFailureUnlocks checks that write errors retain their identity and do not leave the shared lock held.
func (*outputSuite) TestWriteFailureUnlocks(c *tc.C) {
	failure := errors.New("output unavailable")
	var mu sync.Mutex
	writer := lockedWriter{&mu, errorWriter{failure}}
	n, err := writer.Write([]byte("message"))
	c.Check(n, tc.Equals, 0)
	c.Check(err, tc.ErrorIs, failure)

	unlocked := mu.TryLock()
	c.Assert(unlocked, tc.Equals, true)

	mu.Unlock()
	var output bytes.Buffer
	writer.writer = &output
	n, err = writer.Write([]byte("next"))
	c.Assert(err, tc.IsNil)

	c.Check(n, tc.Equals, 4)
	c.Check(output.String(), tc.Equals, "next")
}
