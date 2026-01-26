package overwatch

import (
	"fmt"
	"sync"
	"testing"
)

func TestNewRingBuffer(t *testing.T) {
	t.Run("creates buffer with specified size", func(t *testing.T) {
		rb := NewRingBuffer(100)
		if rb.Cap() != 100 {
			t.Errorf("expected capacity 100, got %d", rb.Cap())
		}
		if rb.Len() != 0 {
			t.Errorf("expected length 0, got %d", rb.Len())
		}
	})

	t.Run("uses default size when zero", func(t *testing.T) {
		rb := NewRingBuffer(0)
		if rb.Cap() != 50 {
			t.Errorf("expected default capacity 50, got %d", rb.Cap())
		}
	})

	t.Run("uses default size when negative", func(t *testing.T) {
		rb := NewRingBuffer(-10)
		if rb.Cap() != 50 {
			t.Errorf("expected default capacity 50, got %d", rb.Cap())
		}
	})
}

func TestRingBufferWrite(t *testing.T) {
	t.Run("writes single line", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Write("line1")

		if rb.Len() != 1 {
			t.Errorf("expected length 1, got %d", rb.Len())
		}

		lines := rb.GetLast(1)
		if len(lines) != 1 || lines[0] != "line1" {
			t.Errorf("expected [line1], got %v", lines)
		}
	})

	t.Run("writes multiple lines", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Write("line1")
		rb.Write("line2")
		rb.Write("line3")

		if rb.Len() != 3 {
			t.Errorf("expected length 3, got %d", rb.Len())
		}

		lines := rb.GetAll()
		expected := []string{"line1", "line2", "line3"}
		if !slicesEqual(lines, expected) {
			t.Errorf("expected %v, got %v", expected, lines)
		}
	})
}

func TestRingBufferOverflow(t *testing.T) {
	t.Run("overwrites oldest entries when full", func(t *testing.T) {
		rb := NewRingBuffer(3)
		rb.Write("line1")
		rb.Write("line2")
		rb.Write("line3")
		rb.Write("line4") // Should overwrite line1
		rb.Write("line5") // Should overwrite line2

		if rb.Len() != 3 {
			t.Errorf("expected length 3 (buffer size), got %d", rb.Len())
		}

		lines := rb.GetAll()
		expected := []string{"line3", "line4", "line5"}
		if !slicesEqual(lines, expected) {
			t.Errorf("expected %v, got %v", expected, lines)
		}
	})

	t.Run("maintains correct order after multiple wraps", func(t *testing.T) {
		rb := NewRingBuffer(3)
		// Write 10 lines to wrap multiple times
		for i := 1; i <= 10; i++ {
			rb.Write(fmt.Sprintf("line%d", i))
		}

		lines := rb.GetAll()
		expected := []string{"line8", "line9", "line10"}
		if !slicesEqual(lines, expected) {
			t.Errorf("expected %v, got %v", expected, lines)
		}
	})

	t.Run("IsFull returns true after overflow", func(t *testing.T) {
		rb := NewRingBuffer(3)
		rb.Write("line1")
		rb.Write("line2")

		if rb.IsFull() {
			t.Error("expected IsFull to be false before overflow")
		}

		rb.Write("line3")
		rb.Write("line4") // Overflow

		if !rb.IsFull() {
			t.Error("expected IsFull to be true after overflow")
		}
	})
}

func TestRingBufferGetLast(t *testing.T) {
	t.Run("returns empty slice for n <= 0", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Write("line1")
		rb.Write("line2")

		lines := rb.GetLast(0)
		if len(lines) != 0 {
			t.Errorf("expected empty slice for n=0, got %v", lines)
		}

		lines = rb.GetLast(-5)
		if len(lines) != 0 {
			t.Errorf("expected empty slice for n=-5, got %v", lines)
		}
	})

	t.Run("returns all lines when n > count", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Write("line1")
		rb.Write("line2")

		lines := rb.GetLast(100)
		expected := []string{"line1", "line2"}
		if !slicesEqual(lines, expected) {
			t.Errorf("expected %v, got %v", expected, lines)
		}
	})

	t.Run("returns last n lines in order", func(t *testing.T) {
		rb := NewRingBuffer(10)
		for i := 1; i <= 5; i++ {
			rb.Write(fmt.Sprintf("line%d", i))
		}

		lines := rb.GetLast(3)
		expected := []string{"line3", "line4", "line5"}
		if !slicesEqual(lines, expected) {
			t.Errorf("expected %v, got %v", expected, lines)
		}
	})

	t.Run("returns correct lines after overflow", func(t *testing.T) {
		rb := NewRingBuffer(5)
		for i := 1; i <= 8; i++ {
			rb.Write(fmt.Sprintf("line%d", i))
		}

		lines := rb.GetLast(3)
		expected := []string{"line6", "line7", "line8"}
		if !slicesEqual(lines, expected) {
			t.Errorf("expected %v, got %v", expected, lines)
		}
	})

	t.Run("returns empty slice from empty buffer", func(t *testing.T) {
		rb := NewRingBuffer(10)
		lines := rb.GetLast(5)
		if len(lines) != 0 {
			t.Errorf("expected empty slice from empty buffer, got %v", lines)
		}
	})
}

func TestRingBufferClear(t *testing.T) {
	t.Run("clears all entries", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Write("line1")
		rb.Write("line2")
		rb.Write("line3")

		rb.Clear()

		if rb.Len() != 0 {
			t.Errorf("expected length 0 after clear, got %d", rb.Len())
		}

		lines := rb.GetAll()
		if len(lines) != 0 {
			t.Errorf("expected empty slice after clear, got %v", lines)
		}
	})

	t.Run("resets IsFull flag", func(t *testing.T) {
		rb := NewRingBuffer(3)
		rb.Write("line1")
		rb.Write("line2")
		rb.Write("line3")
		rb.Write("line4") // Overflow

		if !rb.IsFull() {
			t.Error("expected IsFull to be true before clear")
		}

		rb.Clear()

		if rb.IsFull() {
			t.Error("expected IsFull to be false after clear")
		}
	})

	t.Run("allows new writes after clear", func(t *testing.T) {
		rb := NewRingBuffer(3)
		rb.Write("old1")
		rb.Write("old2")
		rb.Clear()
		rb.Write("new1")
		rb.Write("new2")

		lines := rb.GetAll()
		expected := []string{"new1", "new2"}
		if !slicesEqual(lines, expected) {
			t.Errorf("expected %v, got %v", expected, lines)
		}
	})
}

func TestRingBufferConcurrency(t *testing.T) {
	rb := NewRingBuffer(100)
	iterations := 100
	goroutines := 10

	var wg sync.WaitGroup
	wg.Add(goroutines * 2) // Writers and readers

	// Concurrent writers
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				rb.Write(fmt.Sprintf("goroutine%d-line%d", id, j))
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = rb.GetLast(10)
				_ = rb.Len()
				_ = rb.IsFull()
			}
		}()
	}

	wg.Wait()

	// Buffer should have exactly 100 entries (its capacity)
	if rb.Len() != 100 {
		t.Errorf("expected length 100 after concurrent writes, got %d", rb.Len())
	}
}

func TestRingBufferGetAll(t *testing.T) {
	t.Run("returns all lines in order", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Write("line1")
		rb.Write("line2")
		rb.Write("line3")

		lines := rb.GetAll()
		expected := []string{"line1", "line2", "line3"}
		if !slicesEqual(lines, expected) {
			t.Errorf("expected %v, got %v", expected, lines)
		}
	})

	t.Run("returns empty slice for empty buffer", func(t *testing.T) {
		rb := NewRingBuffer(10)
		lines := rb.GetAll()
		if len(lines) != 0 {
			t.Errorf("expected empty slice, got %v", lines)
		}
	})
}

// slicesEqual compares two string slices for equality
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
