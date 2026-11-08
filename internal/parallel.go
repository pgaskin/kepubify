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
// is the current one, so no new ones are started if n <= 1, or if built by
// TinyGo without a scheduler), and yields the index and result of each item in
// order. At most n results are held while waiting for an earlier one. It stops
// at the first error, when ctx is done, or when the caller stops iterating, and
// returns once the calls in progress have finished.
func Parallel[T, R any](ctx context.Context, n int, items []T, fn func(int, T) (R, error)) func(err *error) iter.Seq2[int, R] {
	return iterErr2(func(yield func(int, R) bool) error {
		n := min(n, len(items)) // don't start more goroutines than the number of items
		if n <= 1 || !parallelGoroutines {
			for i, item := range items {
				if err := ctx.Err(); err != nil {
					return err
				}
				r := parallelCall(fn, i, item)
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
			results = make([]parallelResult[R], len(items))
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
			wg.Go(func() {
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
					results[i] = parallelCall(fn, i, items[i])
					close(done[i])
				}
			})
		}

		for i := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			if next.CompareAndSwap(int64(i), int64(i+1)) {
				results[i] = parallelCall(fn, i, items[i])
			} else {
				<-done[i]
				<-taken
			}
			r := results[i]
			results[i] = parallelResult[R]{}
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
	})
}

// ParallelUnordered is like [Parallel], but yields the results as they
// complete, so a slow item doesn't delay the others. If n > 1, it starts n
// goroutines, and the current one only yields results. At most n results are
// held while waiting for the caller.
func ParallelUnordered[T, R any](ctx context.Context, n int, items []T, fn func(int, T) (R, error)) func(err *error) iter.Seq2[int, R] {
	if min(n, len(items)) <= 1 || !parallelGoroutines {
		return Parallel(ctx, n, items, fn)
	}
	return iterErr2(func(yield func(int, R) bool) error {
		type indexed struct {
			i int
			r parallelResult[R]
		}
		var (
			next    atomic.Int64
			results = make(chan indexed)
			stop    = make(chan struct{})
			wg      sync.WaitGroup
		)
		defer func() {
			close(stop)
			wg.Wait()
		}()

		for range min(n, len(items)) {
			wg.Go(func() {
				for {
					select {
					case <-stop:
						return
					default:
					}
					if ctx.Err() != nil {
						return
					}
					i := int(next.Add(1) - 1)
					if i >= len(items) {
						return
					}
					select {
					case results <- indexed{i, parallelCall(fn, i, items[i])}:
					case <-stop:
						return
					}
				}
			})
		}

		for range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			var x indexed
			select {
			case x = <-results:
			case <-ctx.Done():
				return ctx.Err()
			}
			if x.r.panic != nil {
				panic(x.r.panic)
			}
			if x.r.err != nil {
				return x.r.err
			}
			if !yield(x.i, x.r.val) {
				return nil
			}
		}
		return nil
	})
}

type parallelResult[R any] struct {
	val   R
	err   error
	panic *parallelPanic
}

func parallelCall[T, R any](fn func(int, T) (R, error), i int, item T) (r parallelResult[R]) {
	defer func() {
		if v := recover(); v != nil {
			r.panic = &parallelPanic{v, debug.Stack()}
		}
	}()
	r.val, r.err = fn(i, item)
	return r
}

type parallelPanic struct {
	value any
	stack []byte
}

func (p *parallelPanic) Error() string {
	return fmt.Sprintf("%v\n\n%s", p.value, p.stack)
}

func iterErr2[K, V any](fn func(yield func(K, V) bool) error) func(err *error) iter.Seq2[K, V] {
	return func(err *error) iter.Seq2[K, V] {
		return func(yield func(K, V) bool) {
			*err = fn(yield)
		}
	}
}
