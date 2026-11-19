package app

import (
	"context"
	"testing"
	"time"
)

func TestDescriptionQueueIsFIFOAndDeduplicates(t *testing.T) {
	queue := newDescriptionQueue(4)
	ctx := context.Background()

	for _, id := range []string{"1", "2", "1", "3"} {
		if !queue.push(ctx, id) {
			t.Fatalf("push(%q) = false", id)
		}
	}

	assertQueuePops(t, queue, []string{"1", "2", "3"})
	if queue.len() != 0 {
		t.Errorf("len = %d, want 0", queue.len())
	}
}

func TestDescriptionQueueBlocksWhenFull(t *testing.T) {
	queue := newDescriptionQueue(1)
	if !queue.push(context.Background(), "1") {
		t.Fatal("first push = false")
	}

	pushed := make(chan bool, 1)
	go func() { pushed <- queue.push(context.Background(), "2") }()

	assertChannelQuiet(t, pushed, "push returned while the queue was full")
	assertQueuePops(t, queue, []string{"1"})
	assertChannelTrue(t, pushed, "blocked push did not resume")
	assertQueuePops(t, queue, []string{"2"})
}

func TestDescriptionQueueUnblocksOnCancel(t *testing.T) {
	queue := newDescriptionQueue(1)
	if !queue.push(context.Background(), "1") {
		t.Fatal("first push = false")
	}

	ctx, cancel := context.WithCancel(context.Background())
	pushed := make(chan bool, 1)
	go func() { pushed <- queue.push(ctx, "2") }()
	cancel()

	assertChannelFalse(t, pushed, "cancelled push did not return false")
}

func TestDescriptionQueueClosesAndDrains(t *testing.T) {
	queue := newDescriptionQueue(2)
	ctx := context.Background()

	queue.push(ctx, "1")
	queue.push(ctx, "2")
	queue.close()

	assertQueuePops(t, queue, []string{"1", "2"})
	if _, ok := queue.pop(ctx); ok {
		t.Fatal("pop() succeeded on a drained closed queue")
	}
	if queue.push(ctx, "3") {
		t.Fatal("push() succeeded on a closed queue")
	}
}

// assertQueuePops checks a sequence of FIFO pops.
func assertQueuePops(t *testing.T, queue *descriptionQueue, want []string) {
	t.Helper()

	for _, id := range want {
		got, ok := queue.pop(context.Background())
		if !ok || got != id {
			t.Fatalf("pop() = %q, %v; want %q, true", got, ok, id)
		}
	}
}

// assertChannelQuiet checks that no value arrives within the poll window.
func assertChannelQuiet(t *testing.T, ch <-chan bool, message string) {
	t.Helper()

	select {
	case <-ch:
		t.Fatal(message)
	case <-time.After(50 * time.Millisecond):
	}
}

// assertChannelTrue checks that the channel reports true.
func assertChannelTrue(t *testing.T, ch <-chan bool, message string) {
	t.Helper()

	select {
	case ok := <-ch:
		if !ok {
			t.Fatalf("%s: got false", message)
		}
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}

// assertChannelFalse checks that the channel reports false.
func assertChannelFalse(t *testing.T, ch <-chan bool, message string) {
	t.Helper()

	select {
	case ok := <-ch:
		if ok {
			t.Fatalf("%s: got true", message)
		}
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}
