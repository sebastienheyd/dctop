package docker

import (
	"bufio"
	"context"
	"time"
)

const maxLineSize = 1 << 20

// WatchStats sends every usage sample of `docker stats` to out until ctx is
// done. When the stream ends or cannot start (daemon restart), it is started
// again after the retry delay.
func (c *Client) WatchStats(ctx context.Context, out chan<- Stat) {
	c.watch(ctx, []string{"stats", "--format", "{{json .}}"}, func(line string) bool {
		s, ok := ParseStatLine(line, c.now())
		if !ok {
			return true
		}
		select {
		case out <- s:
			return true
		case <-ctx.Done():
			return false
		}
	})
}

// WatchEvents puts a token in out for every event that should trigger a
// refresh, until ctx is done. It never blocks: when a token is already
// waiting, the new one is dropped, since one refresh covers both.
func (c *Client) WatchEvents(ctx context.Context, out chan<- struct{}) {
	c.watch(ctx, []string{"events", "--format", "{{json .}}"}, func(line string) bool {
		if IsRelevantEvent(line) {
			select {
			case out <- struct{}{}:
			default:
			}
		}
		return true
	})
}

// watch runs a streaming command and hands each line to handle, restarting
// the command after retryDelay whenever it ends, until ctx is done or handle
// returns false.
func (c *Client) watch(ctx context.Context, args []string, handle func(string) bool) {
	for ctx.Err() == nil {
		r, err := c.runner.Stream(ctx, args...)
		if err == nil {
			stop := context.AfterFunc(ctx, func() { r.Close() })
			sc := bufio.NewScanner(r)
			sc.Buffer(make([]byte, 64*1024), maxLineSize)
			for sc.Scan() {
				if !handle(sc.Text()) {
					break
				}
			}
			if stop() {
				r.Close()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.retryDelay):
		}
	}
}
