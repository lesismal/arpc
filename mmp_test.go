// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"math/rand"
	"testing"
)

func TestPoolGet(t *testing.T) {
	var maxMemSize = 1024 * 64
	pool := newMemPool(maxMemSize)
	if pool.Get(0) != nil {
		t.Fatal(0)
	}
	if len(pool.Get(1)) != 1 || cap(pool.Get(1)) != 1 {
		t.Fatal(1)
	}
	if len(pool.Get(maxMemSize)) != maxMemSize || cap(pool.Get(maxMemSize)) != maxMemSize {
		t.Fatal(1)
	}
	if pool.Get(maxMemSize+1) != nil {
		t.Fatal(maxMemSize + 1)
	}

	for i := 0; i < int(pool.definition(maxMemSize)); i++ {
		low := 1<<i + 1
		high := 1 << (i + 1)
		step := (high - low) / 64
		if step == 0 {
			step = 1
		}
		for j := low; j <= high && j <= maxMemSize; j += step {
			if len(pool.Get(j)) != j || cap(pool.Get(j)) != 1<<(i+1) {
				t.Fatalf("[%v, %v, %v, %v, %v]", i, j, low, high, step)
			}
		}
	}
}

func TestPoolPut(t *testing.T) {
	var maxMemSize = 1024 * 64
	pool := newMemPool(maxMemSize)
	if err := pool.Put(nil); err == nil {
		t.Fatal("put nil misbehavior")
	}
	if err := pool.Put(make([]byte, 3, 3)); err == nil {
		t.Fatal("put elem:3 []bytes misbehavior")
	}
	if err := pool.Put(make([]byte, 4, 4)); err != nil {
		t.Fatal("put elem:4 []bytes misbehavior")
	}
	if err := pool.Put(make([]byte, 1023, 1024)); err != nil {
		t.Fatal("put elem:1024 []bytes misbehavior")
	}
	if err := pool.Put(make([]byte, maxMemSize, maxMemSize)); err != nil {
		t.Fatal("put elem:65536 []bytes misbehavior")
	}
	if err := pool.Put(make([]byte, maxMemSize+1, maxMemSize+1)); err == nil {
		t.Fatal("put elem:65537 []bytes misbehavior")
	}
}

func TestPoolPutThenGet(t *testing.T) {
	var maxMemSize = 1024 * 64
	pool := newMemPool(maxMemSize)
	data := pool.Get(4)
	pool.Put(data)
	newData := pool.Get(4)
	if cap(data) != cap(newData) {
		t.Fatal("different cap while pool.Get()")
	}
}

func Benchmark_MemPool_Definition(b *testing.B) {
	rand.Seed(99)
	var maxMemSize = 1024 * 64
	pool := newMemPool(maxMemSize)
	for i := 0; i < b.N; i++ {
		pool.definition(rand.Intn(maxMemSize))
	}
}
