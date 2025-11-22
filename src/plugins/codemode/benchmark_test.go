package codemode

import (
	"reflect"
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

func BenchmarkNewInterpreter(b *testing.B) {
	for i := 0; i < b.N; i++ {
		interp.New(interp.Options{})
	}
}

func BenchmarkUseStdlib(b *testing.B) {
	for i := 0; i < b.N; i++ {
		in := interp.New(interp.Options{})
		in.Use(stdlib.Symbols)
	}
}

func BenchmarkInjectHelpers(b *testing.B) {
	// We need a mock client or nil for this benchmark if we just want to test reflection overhead
	// But injectHelpers takes a client. Let's just pass nil and see if it crashes or if we can mock it easily.
	// Looking at the code, it creates closures capturing the client. It doesn't call methods on it immediately.
	// So passing nil should be fine for benchmarking the injection itself.

	for i := 0; i < b.N; i++ {
		in := interp.New(interp.Options{})
		// We must load stdlib first as injectHelpers might depend on it (though the code says it loads stdlib itself? No, injectHelpers calls i.Use(stdlib.Symbols))
		// Actually, let's look at the code again.
		// func injectHelpers(i *interp.Interpreter, client utcp.UtcpClientInterface) error {
		// 	if err := i.Use(stdlib.Symbols); err != nil { ... }
		// ...
		// }
		// So injectHelpers DOES load stdlib.

		// We can't call injectHelpers directly because it's not exported in this package (it's in codemode package, but we are in codemode package in the test file? No, the test file is package codemode).
		// Wait, the test file I wrote is `package codemode`.
		// But `injectHelpers` is in `codemode.go` which is `package codemode`.
		// So I can call it.

		// However, I need to pass a client.
		// Let's define a dummy client.

		// But wait, I can't easily define a dummy client that satisfies the interface without importing utcp.
		// And I don't want to complicate the benchmark too much.
		// Let's just see if I can call it with nil.
		// The closures capture `client`. They don't access it until called.
		// So nil should be fine.

		_ = injectHelpers(in, nil)
	}
}

func BenchmarkExecuteSimple(b *testing.B) {
	// We need to construct a CodeModeUTCP to call Execute, or just replicate the logic.
	// Let's replicate the logic to avoid complex setup with mocks for now,
	// or better, just use the components we have.

	// We can't easily use CodeModeUTCP.Execute because it needs a client and model.
	// But we can test the core logic: init interpreter, inject helpers, wrap code, eval.

	code := `a := 1; b := 2; __out = a + b`

	for i := 0; i < b.N; i++ {
		interp := interp.New(interp.Options{})
		_ = injectHelpers(interp, nil)

		// We need to wrap the code
		// But wrapIntoProgram is a method of CodeModeUTCP? No, it's a standalone function in the file but might be unexported.
		// It is `func (c *CodeModeUTCP) prepareWrappedProgram(code string)`.
		// And `func wrapIntoProgram(clean string) string`.
		// `wrapIntoProgram` is unexported but package-level. So we can call it.
		// `preprocessUserCode` is also package-level unexported.

		processed := preprocessUserCode(code)
		clean := normalizeSnippet(processed)
		wrapped := wrapIntoProgram(clean)

		_, _ = interp.Eval(wrapped)
		_, _ = interp.Eval(`main.run()`)
	}
}

func TestFmtUsage(t *testing.T) {
	// This test checks if we can use fmt without explicit import (since imports are stripped)
	// If imports are stripped, and we can't use fmt, then codemode is very limited.

	code := `fmt.Println("hello")`

	// We need to replicate the execution logic
	interp := interp.New(interp.Options{})
	_ = injectHelpers(interp, nil)

	processed := preprocessUserCode(code)
	// processed will be `fmt.Println("hello")` (imports stripped if any, but here none)
	// wait, fixBareReturn and ensureOutAssigned will modify it.
	// ensureOutAssigned will make it `__out = fmt.Println("hello")`

	clean := normalizeSnippet(processed)
	wrapped := wrapIntoProgram(clean)

	_, err := interp.Eval(wrapped)
	if err != nil {
		t.Logf("Compilation failed: %v", err)
		// If it fails with "undefined: fmt", then my hypothesis is correct.
	} else {
		_, err = interp.Eval(`main.run()`)
		if err != nil {
			t.Logf("Execution failed: %v", err)
		} else {
			t.Logf("Execution succeeded!")
		}
	}
}

func BenchmarkPartialLoad(b *testing.B) {
	// Create a subset of symbols
	subset := make(map[string]map[string]reflect.Value)
	if val, ok := stdlib.Symbols["context/context"]; ok {
		subset["context/context"] = val
	}

	for i := 0; i < b.N; i++ {
		in := interp.New(interp.Options{})
		in.Use(subset)
	}
}
