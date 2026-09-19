// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"
	"strings"
	"testing"
)

// capture runs f with Output redirected, and returns what was written.
func capture(f func()) string {
	var buf bytes.Buffer
	old := Output
	Output = &buf
	defer func() { Output = old }()
	f()
	return buf.String()
}

func TestLevels(t *testing.T) {
	old := DefaultLogger
	defer SetLogger(old)
	SetLogger(&logger{level: LevelInfo})

	logAll := func() {
		Debug("d %v", 1)
		Info("i %v", 2)
		Warn("w %v", 3)
		Error("e %v", 4)
	}

	for _, tc := range []struct {
		level int
		want  []string
	}{
		{LevelAll, []string{"[DBG] d 1", "[INF] i 2", "[WRN] w 3", "[ERR] e 4"}},
		{LevelDebug, []string{"[DBG] d 1", "[INF] i 2", "[WRN] w 3", "[ERR] e 4"}},
		{LevelInfo, []string{"[INF] i 2", "[WRN] w 3", "[ERR] e 4"}},
		{LevelWarn, []string{"[WRN] w 3", "[ERR] e 4"}},
		{LevelError, []string{"[ERR] e 4"}},
		{LevelNone, nil},
	} {
		SetLevel(tc.level)
		out := capture(logAll)
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if out == "" {
			lines = nil
		}
		if len(lines) != len(tc.want) {
			t.Fatalf("level %v: got %q", tc.level, out)
		}
		for i, want := range tc.want {
			if !strings.HasSuffix(lines[i], want) {
				t.Fatalf("level %v: line %q, want suffix %q", tc.level, lines[i], want)
			}
		}
	}
}

func TestInvalidLevel(t *testing.T) {
	old := DefaultLogger
	defer SetLogger(old)
	l := &logger{level: LevelWarn}
	SetLogger(l)

	if out := capture(func() { SetLevel(100) }); !strings.Contains(out, "invalid log level: 100") {
		t.Fatalf("SetLevel(100) wrote %q", out)
	}
	if out := capture(func() { l.SetLevel(-1) }); !strings.Contains(out, "invalid log level: -1") {
		t.Fatalf("logger.SetLevel(-1) wrote %q", out)
	}
	if l.level != LevelWarn {
		t.Fatal("an invalid level should be ignored")
	}
}

func TestNilLogger(t *testing.T) {
	old := DefaultLogger
	defer SetLogger(old)
	SetLogger(nil)
	out := capture(func() {
		Debug("d")
		Info("i")
		Warn("w")
		Error("e")
	})
	if out != "" {
		t.Fatalf("a nil logger wrote %q", out)
	}
}
