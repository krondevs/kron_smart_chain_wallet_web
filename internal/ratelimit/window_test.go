package ratelimit

import (
	"testing"
	"time"
)

func TestWindow(t *testing.T) {
	w := NewWindow(2, time.Minute)
	defer w.Stop()
	ok, remaining, _ := w.Allow("a")
	if !ok || remaining != 1 {
		t.Fatalf("first allow = %v remaining=%d", ok, remaining)
	}
	ok, remaining, _ = w.Allow("a")
	if !ok || remaining != 0 {
		t.Fatalf("second allow = %v remaining=%d", ok, remaining)
	}
	ok, remaining, reset := w.Allow("a")
	if ok || remaining != 0 || reset <= 0 {
		t.Fatalf("third allow = %v remaining=%d reset=%s", ok, remaining, reset)
	}
	if ok, _, _ := w.Allow("b"); !ok {
		t.Fatal("different key should have its own window")
	}
}
