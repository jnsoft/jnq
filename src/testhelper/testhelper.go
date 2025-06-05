package testhelper

import "testing"

func AssertNil(t *testing.T, got any) {
	t.Helper()
	if got != nil {
		t.Errorf("Assert Nil failed")
	}
}

func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func AssertTrue(t *testing.T, got bool) {
	t.Helper()
	if !got {
		t.Errorf("Assert True failed")
	}
}

func AssertFalse(t *testing.T, got bool) {
	t.Helper()
	if got {
		t.Errorf("Assert False failed")
	}
}

func AssertEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func AssertNotEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got == want {
		t.Errorf("didn't want %v", got)
	}
}

func CollectionAssertEqual[T comparable](t *testing.T, got, want []T) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("got length %v, want length %v", len(got), len(want))
		return
	}
	for i := range want {
		if got[i] != want[i] {
			if len(got) > 20 {
				t.Errorf("collection assert failed at index %v", i)
				return
			} else {
				t.Errorf("got %v, want %v", got, want)
				return
			}
		}
	}
}
