package internal

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallel(t *testing.T) {
	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}
	square := func(i, x int) (int, error) {
		if i != x {
			return 0, fmt.Errorf("item %d: got index %d", x, i)
		}
		if x%7 == 0 {
			time.Sleep(time.Millisecond) // vary the completion order
		}
		return x * x, nil
	}
	tracked := func(t *testing.T, fn func(int, int) (int, error)) (func(int, int) (int, error), func()) {
		var running, started atomic.Int64
		return func(i, x int) (int, error) {
				started.Add(1)
				running.Add(1)
				defer running.Add(-1)
				return fn(i, x)
			}, func() {
				if n := running.Load(); n != 0 {
					t.Errorf("expected no calls to be running after the iteration, got %d", n)
				}
				n := started.Load()
				time.Sleep(10 * time.Millisecond)
				if m := started.Load(); m != n {
					t.Errorf("expected no calls to be started after the iteration, got %d", m-n)
				}
			}
	}
	for _, n := range []int{0, 1, 2, 3, 8, 200} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			t.Run("Simple", func(t *testing.T) {
				var err error
				var got []int
				for i, r := range Parallel(context.Background(), n, items, square)(&err) {
					if i != len(got) {
						t.Fatalf("expected index %d, got %d", len(got), i)
					}
					got = append(got, r)
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(got) != len(items) {
					t.Fatalf("expected %d results, got %d", len(items), len(got))
				}
				for i, r := range got {
					if r != i*i {
						t.Fatalf("result %d: expected %d, got %d (results must be in order)", i, i*i, r)
					}
				}
			})

			t.Run("Error", func(t *testing.T) {
				errTest := errors.New("test")
				fn, check := tracked(t, func(i, x int) (int, error) {
					if x == 10 {
						return 0, errTest
					}
					return square(i, x)
				})
				var err error
				var got []int
				for _, r := range Parallel(context.Background(), n, items, fn)(&err) {
					got = append(got, r)
				}
				check()
				if !errors.Is(err, errTest) {
					t.Fatalf("expected error, got %v", err)
				}
				if len(got) != 10 {
					t.Fatalf("expected the results before the error, got %d", len(got))
				}
			})

			t.Run("Break", func(t *testing.T) {
				fn, check := tracked(t, square)
				var err error
				var got []int
				for _, r := range Parallel(context.Background(), n, items, fn)(&err) {
					if got = append(got, r); len(got) == 5 {
						break
					}
				}
				check()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(got) != 5 {
					t.Fatalf("expected 5 results, got %d", len(got))
				}
			})

			t.Run("ContextCancel", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				fn, check := tracked(t, square)
				var err error
				var got []int
				for _, r := range Parallel(ctx, n, items, fn)(&err) {
					if got = append(got, r); len(got) == 5 {
						cancel()
					}
				}
				check()
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected context error, got %v", err)
				}
				if len(got) != 5 {
					t.Fatalf("expected 5 results, got %d", len(got))
				}
			})

			t.Run("ContextCancelled", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				var err error
				for range Parallel(ctx, n, items, square)(&err) {
					t.Fatal("expected no results")
				}
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected context error, got %v", err)
				}
			})

			t.Run("Empty", func(t *testing.T) {
				var err error
				for range Parallel(context.Background(), n, nil, square)(&err) {
					t.Fatal("expected no results")
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			})

			t.Run("Panic", func(t *testing.T) {
				// delay the first item until the second has started so the
				// panic happens on another goroutine if there is one
				second := make(chan struct{})
				fn, check := tracked(t, func(i, x int) (int, error) {
					switch x {
					case 0:
						if n > 1 {
							<-second
						}
					case 1:
						close(second)
						panic("test")
					}
					return square(i, x)
				})
				defer func() {
					v := recover()
					check()
					p, ok := v.(*parallelPanic)
					if !ok || p.value != "test" {
						t.Fatalf("expected the panic to be rethrown, got %v", v)
					}
					if !strings.Contains(string(p.stack), "parallel_test.go") {
						t.Fatalf("expected the stack of the panic, got:\n%s", p.stack)
					}
				}()
				var err error
				for range Parallel(context.Background(), n, items, fn)(&err) {
				}
				t.Fatal("expected a panic")
			})

			t.Run("LoopPanic", func(t *testing.T) {
				fn, check := tracked(t, square)
				defer func() {
					v := recover()
					check()
					if v != "loop" {
						t.Fatalf("expected the panic to propagate, got %v", v)
					}
				}()
				var err error
				for _, r := range Parallel(context.Background(), n, items, fn)(&err) {
					if r == 25 {
						panic("loop")
					}
				}
				t.Fatal("expected a panic")
			})
		})
	}

	t.Run("Bounded", func(t *testing.T) {
		var started atomic.Int64
		release := make(chan struct{})
		observed := make(chan int64, 1)
		go func() {
			// the first result is delayed, so nothing can be yielded, and no
			// more than n items should be active
			time.Sleep(50 * time.Millisecond)
			observed <- started.Load()
			close(release)
		}()
		var err error
		var got int
		for range Parallel(context.Background(), 4, items, func(i, _ int) (int, error) {
			started.Add(1)
			if i == 0 {
				<-release
			}
			return i, nil
		})(&err) {
			got++
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != len(items) {
			t.Fatalf("expected %d results, got %d", len(items), got)
		}
		if n := <-observed; n > 4 {
			t.Fatalf("expected at most 4 items to be started while the first was delayed, got %d", n)
		}
	})
}
