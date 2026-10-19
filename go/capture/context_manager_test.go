package capture

import (
	"testing"
	"time"
)

func TestContextEventHandlersWaitBlocksUntilHandlerFinishes(t *testing.T) {
	handlers := &ContextEventHandlers{}
	started := make(chan struct{})
	release := make(chan struct{})

	handlers.run(func() {
		close(started)
		<-release
	})
	<-started

	waited := make(chan struct{})
	go func() {
		handlers.Wait()
		close(waited)
	}()

	select {
	case <-waited:
		t.Fatal("Wait returned before the handler finished")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)

	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after the handler finished")
	}
}

func TestContextEventHandlersWaitRejectsNewWork(t *testing.T) {
	handlers := &ContextEventHandlers{}
	handlers.Wait()

	ran := make(chan struct{}, 1)
	handlers.run(func() {
		ran <- struct{}{}
	})

	select {
	case <-ran:
		t.Fatal("run executed work after Wait")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestContextEventHandlersRecoversHandlerPanics(t *testing.T) {
	handlers := &ContextEventHandlers{}
	handlers.run(func() {
		panic("boom")
	})

	waited := make(chan struct{})
	go func() {
		handlers.Wait()
		close(waited)
	}()

	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after a handler panic")
	}
}
