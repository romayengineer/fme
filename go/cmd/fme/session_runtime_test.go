package main

import (
	"testing"

	"github.com/romayengineer/fme/go/browser/driver"
)

// fakePlaywrightRuntime records Stop calls. The embedded interface satisfies
// the rest of the runtime surface; only Stop is exercised here.
type fakePlaywrightRuntime struct {
	driver.PlaywrightWrapperInterface
	stopCalls int
}

func (f *fakePlaywrightRuntime) Stop() error {
	f.stopCalls++
	return nil
}

func TestBrowserSessionFactoryReusesRuntime(t *testing.T) {
	runtime := &fakePlaywrightRuntime{}
	created := 0
	factory := &browserSessionFactory{
		newPlaywright: func() (driver.PlaywrightWrapperInterface, error) {
			created++
			return runtime, nil
		},
	}

	first, err := factory.runtime()
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	second, err := factory.runtime()
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	if created != 1 || first != second {
		t.Errorf("created = %d, same = %v; want one shared runtime", created, first == second)
	}

	assertCloseStopsRuntime(t, factory, runtime, 1)
	assertCloseStopsRuntime(t, factory, runtime, 1)
}

// assertCloseStopsRuntime closes the factory and checks the cumulative stops.
func assertCloseStopsRuntime(t *testing.T, factory *browserSessionFactory, runtime *fakePlaywrightRuntime, want int) {
	t.Helper()

	if err := factory.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if runtime.stopCalls != want {
		t.Errorf("Stop calls = %d, want %d", runtime.stopCalls, want)
	}
}
