package mustache

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// runzero regression tests. A self-referential or
// cyclic partial provider would, without the depth cap, recurse until the
// goroutine stack overflows — a fatal, unrecoverable runtime error. We assert
// the renderer returns an error BEFORE that happens (the test must not rely on
// recover(), which cannot catch a stack overflow).

type selfRefProvider struct{ body string }

func (p selfRefProvider) Get(_ string) (string, error) { return p.body, nil }

// TestSelfReferentialPartialDepthCap mirrors the event-template bodyPartialProvider:
// the provider returns the same body for any partial name, and that body
// references a partial, producing infinite expansion absent the depth cap.
func TestSelfReferentialPartialDepthCap(t *testing.T) {
	c := New().WithEscapeMode(EscapeHTML)
	c.WithPartials(selfRefProvider{body: "X{{> body }}"})
	tmpl, err := c.CompileString("start {{> body }} end")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	_, err = tmpl.Render(map[string]any{})
	if err == nil {
		t.Fatal("expected a depth-limit error for self-referential partial, got nil")
	}
	if !errors.Is(err, ErrMaxPartialDepthExceeded) {
		t.Fatalf("expected ErrMaxPartialDepthExceeded, got: %v", err)
	}
}

// TestPartialDepthResetsBetweenRenders confirms the per-render depth counter is
// reset, so a finite partial chain can render repeatedly.
func TestPartialDepthResetsBetweenRenders(t *testing.T) {
	c := New().WithEscapeMode(Raw)
	c.WithPartials(&StaticProvider{Partials: map[string]string{"p": "hello"}})
	tmpl, err := c.CompileString("{{> p}}")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for i := 0; i < 3; i++ {
		out, err := tmpl.Render(map[string]any{})
		if err != nil {
			t.Fatalf("render %d: %v", i, err)
		}
		if out != "hello" {
			t.Fatalf("render %d: got %q", i, out)
		}
	}
}

// TestOutputSizeCap confirms a configured output-size cap aborts a render that
// would otherwise emit too much (e.g. a billion-laughs style expansion).
func TestOutputSizeCap(t *testing.T) {
	c := New().WithEscapeMode(Raw).WithRenderLimits(100, 0)
	// A section iterating over many items, each emitting text, exceeds the cap.
	tmpl, err := c.CompileString("{{#items}}XXXXXXXXXX{{/items}}")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	items := make([]map[string]any, 50)
	_, err = tmpl.Render(map[string]any{"items": items})
	if err == nil {
		t.Fatal("expected output-size error, got nil")
	}
	if !errors.Is(err, ErrOutputSizeExceeded) {
		t.Fatalf("expected ErrOutputSizeExceeded, got: %v", err)
	}
}

// TestRenderDeadline confirms a render deadline aborts a slow render. We pair a
// self-referential partial (lots of renderTemplate invocations) with a tiny
// deadline; either the deadline or the depth cap must stop it with an error.
func TestRenderDeadline(t *testing.T) {
	c := New().WithEscapeMode(EscapeHTML).WithRenderLimits(0, time.Nanosecond)
	c.WithPartials(selfRefProvider{body: "X{{> body }}"})
	tmpl, err := c.CompileString("start {{> body }} end")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	_, err = tmpl.Render(map[string]any{})
	if err == nil {
		t.Fatal("expected an error from deadline/depth guard, got nil")
	}
	if !errors.Is(err, ErrRenderDeadlineExceeded) && !errors.Is(err, ErrMaxPartialDepthExceeded) {
		t.Fatalf("expected deadline or depth error, got: %v", err)
	}
}

// TestNormalRenderUnaffected guards against the limits breaking ordinary output.
func TestNormalRenderUnaffected(t *testing.T) {
	c := New().WithEscapeMode(Raw)
	tmpl, err := c.CompileString("Hello {{name}}!")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	out, err := tmpl.Render(map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "Hello world!") {
		t.Fatalf("unexpected output: %q", out)
	}
}
