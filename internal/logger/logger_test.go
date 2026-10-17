package logger

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// captureStdout replaces os.Stdout with a pipe for the duration of fn and
// returns whatever was written. The logger under test writes to os.Stdout
// directly via slog.NewJSONHandler / NewTextHandler, so this is the only way
// to observe its output without changing the production API.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()
	_ = w.Close()
	os.Stdout = orig
	<-done
	return buf.String()
}

func TestNew_ReturnsLogger(t *testing.T) {
	if l := New("info", "json"); l == nil {
		t.Fatal("New returned nil")
	}
}

func TestNew_Levels(t *testing.T) {
	cases := []struct {
		level    string
		call     func(*slog.Logger) // emit at a level that matches the threshold
		wantText string
	}{
		{"debug", func(l *slog.Logger) { l.Debug("hi") }, "level=DEBUG"},
		{"info", func(l *slog.Logger) { l.Info("hi") }, "level=INFO"},
		{"warn", func(l *slog.Logger) { l.Warn("hi") }, "level=WARN"},
		{"error", func(l *slog.Logger) { l.Error("hi") }, "level=ERROR"},
		{"unknown", func(l *slog.Logger) { l.Info("hi") }, "level=INFO"}, // unknown falls through to default (info)
		{"", func(l *slog.Logger) { l.Info("hi") }, "level=INFO"},        // empty falls through to default
	}
	for _, c := range cases {
		t.Run(c.level, func(t *testing.T) {
			// Construct the logger INSIDE captureStdout so the handler picks up
			// the swapped os.Stdout pipe writer — not the real one.
			out := captureStdout(t, func() {
				l := New(c.level, "text")
				c.call(l)
			})
			if !strings.Contains(out, c.wantText) {
				t.Errorf("level=%q: expected %q in output, got %q", c.level, c.wantText, out)
			}
		})
	}
}

func TestNew_FiltersBelowThreshold(t *testing.T) {
	cases := []struct {
		level      string
		mustAppear []string
		mustMiss   []string
	}{
		{"debug", []string{"d:", "i:", "w:", "e:"}, nil},
		{"info", []string{"i:", "w:", "e:"}, []string{"d:"}},
		{"warn", []string{"w:", "e:"}, []string{"d:", "i:"}},
		{"error", []string{"e:"}, []string{"d:", "i:", "w:"}},
	}
	for _, c := range cases {
		t.Run(c.level, func(t *testing.T) {
			out := captureStdout(t, func() {
				l := New(c.level, "text")
				l.Debug("d: debug")
				l.Info("i: info")
				l.Warn("w: warn")
				l.Error("e: error")
			})
			for _, s := range c.mustAppear {
				if !strings.Contains(out, s) {
					t.Errorf("level=%s: %q missing from %q", c.level, s, out)
				}
			}
			for _, s := range c.mustMiss {
				if strings.Contains(out, s) {
					t.Errorf("level=%s: %q should be filtered, but appears in %q", c.level, s, out)
				}
			}
		})
	}
}

func TestNew_JSONFormat(t *testing.T) {
	out := captureStdout(t, func() {
		l := New("info", "json")
		l.Info("hello", "user", "alice")
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v\nraw: %q", err, out)
	}
	if parsed["msg"] != "hello" {
		t.Errorf("msg: %v", parsed["msg"])
	}
	if parsed["user"] != "alice" {
		t.Errorf("user: %v", parsed["user"])
	}
	if parsed["level"] != "INFO" {
		t.Errorf("level: %v", parsed["level"])
	}
}

func TestNew_TextFormat(t *testing.T) {
	out := captureStdout(t, func() {
		l := New("info", "text")
		l.Info("hello", "user", "alice")
	})
	if !strings.Contains(out, "level=INFO") {
		t.Errorf("missing level=INFO in: %q", out)
	}
	if !strings.Contains(out, "user=alice") {
		t.Errorf("missing user=alice in: %q", out)
	}
}
