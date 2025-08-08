package resizer

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestResize_Shrink_Stress(t *testing.T) {
	const startCap = 64
	const target = 16
	const iters = 300

	for it := 0; it < iters; it++ {
		fill := target + 1 + (it % (startCap - target))
		ch := make(chan int, startCap)
		for i := 0; i < fill; i++ {
			ch <- i
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func(iter int) {
			defer wg.Done()
			for j := 0; j < fill-target; j++ {
				got := <-ch
				if got != j {
					t.Fatalf("iter=%d reader order: got=%d want=%d", iter, got, j)
				}
			}
		}(it)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := Resize[int](&ch, target, ctx); err != nil {
			cancel()
			t.Fatalf("iter=%d Resize shrink failed: %v", it, err)
		}
		cancel()

		if cap(ch) != target {
			t.Fatalf("iter=%d cap after shrink = %d, want %d", it, cap(ch), target)
		}
		if l := len(ch); l != target {
			t.Fatalf("iter=%d len after shrink = %d, want %d", it, l, target)
		}

		rem := make([]int, 0, target)
		for len(rem) < target {
			rem = append(rem, <-ch)
		}
		for i, v := range rem {
			want := (fill - target) + i
			if v != want {
				t.Fatalf("iter=%d remainder order: got=%d want=%d idx=%d", it, v, want, i)
			}
		}
		wg.Wait()
	}
}

func TestResize_Grow_Stress(t *testing.T) {
	const newCap = 32
	const iters = 300

	for it := 0; it < iters; it++ {
		startCap := 4 + (it % 4)
		toSend := 10 + (it % 16)

		ch := make(chan int, startCap)

		var wg sync.WaitGroup
		wg.Add(1)
		go func(iter int) {
			defer wg.Done()
			for i := 0; i < toSend; i++ {
				ch <- i
			}
		}(it)

		deadline := time.Now().Add(2 * time.Second)
		for len(ch) < startCap {
			if time.Now().After(deadline) {
				t.Fatalf("iter=%d timeout waiting channel to fill; len=%d", it, len(ch))
			}
			time.Sleep(100 * time.Microsecond)
		}

		if err := Resize[int](&ch, uint(newCap), context.Background()); err != nil {
			t.Fatalf("iter=%d Resize grow failed: %v", it, err)
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("iter=%d sender did not finish after growth", it)
		}

		if cap(ch) != newCap {
			t.Fatalf("iter=%d cap after grow = %d, want %d", it, cap(ch), newCap)
		}
		if l := len(ch); l != toSend {
			t.Fatalf("iter=%d len after grow = %d, want %d", it, l, toSend)
		}

		for i := 0; i < toSend; i++ {
			got := <-ch
			if got != i {
				t.Fatalf("iter=%d grow order mismatch at %d: got=%d want=%d", it, i, got, i)
			}
		}
	}
}

func TestResize_NoChange_Stress(t *testing.T) {
	const iters = 300

	for it := 0; it < iters; it++ {
		cap0 := 8 + (it % 5)
		fill := 3 + (it % 4)

		ch := make(chan int, cap0)
		for i := 0; i < fill; i++ {
			ch <- i
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := Resize[int](&ch, uint(cap0), ctx); err != nil {
			cancel()
			t.Fatalf("iter=%d Resize no-change failed: %v", it, err)
		}
		cancel()

		if cap(ch) != cap0 {
			t.Fatalf("iter=%d cap changed: got=%d want=%d", it, cap(ch), cap0)
		}
		if len(ch) != fill {
			t.Fatalf("iter=%d len changed: got=%d want=%d", it, len(ch), fill)
		}

		for i := 0; i < fill; i++ {
			got := <-ch
			if got != i {
				t.Fatalf("iter=%d no-change order mismatch at %d: got=%d want=%d", it, i, got, i)
			}
		}
	}
}
