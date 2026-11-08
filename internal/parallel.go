package internal

import (
	"context"
	"fmt"
	"iter"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// Parallel calls fn for each item and its index on n goroutines (one of which
// is the current one, so no new ones are started if n <= 1), and yields the
// index and result of each item in order. At most n results are held while
// waiting for an earlier one. It stops at the first error, when ctx is done, or
// when the caller stops iterating, and returns once the calls in progress have
// finished.
func Parallel[T, R any](ctx context.Context, n int, items []T, fn func(int, T) (R, error)) func(err *error) iter.Seq2[int, R] {
	type result struct {
		val   R
		err   error
		panic *parallelPanic
	}
	call := func(i int, item T) (r result) {
		defer func() {
			if v := recover(); v != nil {
				r.panic = &parallelPanic{v, debug.Stack()}
			}
		}()
		r.val, r.err = fn(i, item)
		return r
	}
	return func(err *error) iter.Seq2[int, R] {
		return func(yield func(int, R) bool) {
			*err = func() (err error) {
				n := min(n, len(items)) // don't start more goroutines than the number of items
				if n <= 1 {
					for i, item := range items {
						if err := ctx.Err(); err != nil {
							return err
						}
						r := call(i, item)
						if r.panic != nil {
							panic(r.panic)
						}
						if r.err != nil {
							return r.err
						}
						if !yield(i, r.val) {
							return nil
						}
					}
					return nil
				}

				var (
					next    atomic.Int64
					results = make([]result, len(items))
					done    = make([]chan struct{}, len(items))
					taken   = make(chan struct{}, n-1) // held by items from other goroutines until yielded
					stop    = make(chan struct{})
					wg      sync.WaitGroup
				)
				for i := range done {
					done[i] = make(chan struct{})
				}
				defer func() {
					close(stop)
					next.Store(int64(len(items))) // nothing more is claimed
					wg.Wait()
				}()

				for range n - 1 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						for {
							select {
							case taken <- struct{}{}:
							case <-stop:
								return
							}
							i := int(next.Add(1) - 1)
							if i >= len(items) {
								return
							}
							results[i] = call(i, items[i])
							close(done[i])
						}
					}()
				}

				for i := range items {
					if err := ctx.Err(); err != nil {
						return err
					}
					if next.CompareAndSwap(int64(i), int64(i+1)) {
						results[i] = call(i, items[i])
					} else {
						<-done[i]
						<-taken
					}
					r := results[i]
					results[i] = result{}
					if r.panic != nil {
						panic(r.panic)
					}
					if r.err != nil {
						return r.err
					}
					if !yield(i, r.val) {
						return nil
					}
				}
				return nil
			}()
		}
	}
}

type parallelPanic struct {
	value any
	stack []byte
}

func (p *parallelPanic) Error() string {
	return fmt.Sprintf("%v\n\n%s", p.value, p.stack)
}
