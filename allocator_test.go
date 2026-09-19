// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"bytes"
	"os"
	"testing"
)

func TestNewBufferPool(t *testing.T) {
	bp := New(0, 0).(*BufferPool)
	if bp.smallSize != 64 || bp.bigSize != 64*1024 || bp.smallPool == bp.bigPool {
		t.Fatalf("defaults: small %v, big %v", bp.smallSize, bp.bigSize)
	}
	bp = New(128, 64).(*BufferPool)
	if bp.bigSize != 128 || bp.smallPool != bp.bigPool {
		t.Fatal("bigSize should be at least smallSize, sharing one pool when equal")
	}
}

func testBufferPool(t *testing.T, debug bool) {
	bp := New(8, 32).(*BufferPool)
	bp.Debug = debug

	// Malloc from both pools, growing pooled buffers as needed.
	for _, size := range []int{1, 8, 31, 32, 100} {
		b := bp.Malloc(size)
		if len(b) != size || cap(b) < size {
			t.Fatalf("Malloc(%v): len %v cap %v", size, len(b), cap(b))
		}
		bp.Free(b)
	}

	// Realloc within the capacity reslices.
	b := bp.Malloc(4)
	copy(b, "abcd")
	if r := bp.Realloc(b, 2); string(r) != "ab" {
		t.Fatalf("Realloc shrink = %q", r)
	}
	// Growing below bigSize keeps the content.
	b = bp.Realloc(b, 20)
	if len(b) != 20 || string(b[:4]) != "abcd" {
		t.Fatalf("Realloc small = %q", b[:4])
	}
	// Growing to bigSize moves to the big pool.
	b = bp.Realloc(b, 40)
	if len(b) != 40 || string(b[:4]) != "abcd" {
		t.Fatalf("Realloc big = %q", b[:4])
	}
	// Growing a big buffer beyond its capacity.
	b = bp.Realloc(b, cap(b)+10)
	if string(b[:4]) != "abcd" {
		t.Fatalf("Realloc bigger = %q", b[:4])
	}
	bp.Free(b)

	// Append below bigSize, then across it.
	b = bp.Malloc(0)
	b = bp.Append(b, []byte("0123")...)
	b = bp.AppendString(b, "4567")
	if string(b) != "01234567" {
		t.Fatalf("Append = %q", b)
	}
	b = bp.AppendString(b, string(bytes.Repeat([]byte("x"), 40)))
	if len(b) != 48 || string(b[:8]) != "01234567" {
		t.Fatalf("Append across bigSize: len %v, %q", len(b), b[:8])
	}
	b = bp.Append(b, bytes.Repeat([]byte("y"), cap(b))...)
	if string(b[:8]) != "01234567" {
		t.Fatalf("Append big = %q", b[:8])
	}
	bp.Free(b)

	if debug {
		if len(bp.allocStacks) != 0 || bp.allocCnt != bp.freeCnt {
			t.Fatalf("leaks: %v stacks, alloc %v, free %v", len(bp.allocStacks), bp.allocCnt, bp.freeCnt)
		}
	}
}

func TestBufferPool(t *testing.T) {
	t.Run("Normal", func(t *testing.T) { testBufferPool(t, false) })
	t.Run("Debug", func(t *testing.T) { testBufferPool(t, true) })
}

func TestBufferPool_Debug(t *testing.T) {
	bp := New(8, 32).(*BufferPool)
	bp.Debug = true

	b := bp.Malloc(4)
	if len(bp.allocStacks) != 1 {
		t.Fatal("Malloc should be tracked")
	}
	mustPanic(t, "free of an untracked buffer", func() { bp.Free(make([]byte, 4)) })
	mustPanic(t, "realloc of a zero cap buffer", func() { bp.Realloc(nil, 4) })
	mustPanic(t, "append to a zero cap buffer", func() { bp.AppendString(nil, "x") })
	mustPanic(t, "pointer of a zero cap buffer", func() { getBufferPtr(nil) })

	// A buffer growing in place keeps its record.
	b = bp.AppendString(b[:0], "ab")
	if len(bp.allocStacks) != 1 {
		t.Fatal("in-place append should keep the record")
	}

	withStdoutDiscarded(t, bp.LogDebugInfo)
	bp.Free(b)
}

func TestNativeAllocator(t *testing.T) {
	a := &NativeAllocator{}
	b := a.Malloc(4)
	copy(b, "abcd")
	if r := a.Realloc(b, 2); string(r) != "ab" {
		t.Fatalf("Realloc shrink = %q", r)
	}
	if r := a.Realloc(b, 8); len(r) != 8 || string(r[:4]) != "abcd" {
		t.Fatalf("Realloc grow = %q", r)
	}
	a.Free(b)
}

func TestDefaultAllocatorFuncs(t *testing.T) {
	b := Malloc(2)
	b = Realloc(b, 4)
	b = Append(b[:0], 'a')
	b = AppendString(b, "b")
	if string(b) != "ab" {
		t.Fatalf("got %q", b)
	}
	Free(b)

	SetDebug(true)
	b = Malloc(2)
	withStdoutDiscarded(t, LogDebugInfo)
	Free(b)
	SetDebug(false)

	// They do nothing for an Allocator other than a BufferPool.
	old := DefaultAllocator
	DefaultAllocator = &nativeFullAllocator{}
	defer func() { DefaultAllocator = old }()
	SetDebug(true)
	LogDebugInfo()
}

// nativeFullAllocator completes NativeAllocator into an Allocator.
type nativeFullAllocator struct{ NativeAllocator }

func (nativeFullAllocator) Append(b []byte, more ...byte) []byte   { return append(b, more...) }
func (nativeFullAllocator) AppendString(b []byte, s string) []byte { return append(b, s...) }

// withStdoutDiscarded runs f with os.Stdout redirected to the null device.
func withStdoutDiscarded(t *testing.T, f func()) {
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	old := os.Stdout
	os.Stdout = null
	defer func() { os.Stdout = old }()
	f()
}
