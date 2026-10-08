package calc

import (
	"errors"
	"math"
	"testing"
)

func TestOperations(t *testing.T) {
	const max, min = math.MaxInt64, math.MinInt64

	tests := []struct {
		name    string
		fn      func(a, b int64) (int64, error)
		a, b    int64
		want    int64
		wantErr error
	}{
		{"add", Add, 4, 1, 5, nil},
		{"add negatives", Add, -4, -6, -10, nil},
		{"add to max", Add, max - 1, 1, max, nil},
		{"add overflow", Add, max, 1, 0, ErrOverflow},
		{"add underflow", Add, min, -1, 0, ErrOverflow},

		{"sub spec example", Sub, 4, 1, 3, nil},
		{"sub below zero", Sub, 1, 4, -3, nil},
		{"sub to min", Sub, min + 1, 1, min, nil},
		{"sub overflow", Sub, max, -1, 0, ErrOverflow},
		{"sub underflow", Sub, min, 1, 0, ErrOverflow},

		{"mul", Mul, 6, 7, 42, nil},
		{"mul by zero", Mul, max, 0, 0, nil},
		{"mul signs", Mul, -3, 5, -15, nil},
		{"mul min by one", Mul, min, 1, min, nil},
		{"mul overflow", Mul, max, 2, 0, ErrOverflow},
		{"mul min by minus one", Mul, min, -1, 0, ErrOverflow},
		{"mul minus one by min", Mul, -1, min, 0, ErrOverflow},
		{"mul large overflow", Mul, 1 << 32, 1 << 32, 0, ErrOverflow},

		{"div exact", Div, 8, 2, 4, nil},
		{"div truncates", Div, 7, 2, 3, nil},
		{"div truncates toward zero", Div, -7, 2, -3, nil},
		{"div by zero", Div, 1, 0, 0, ErrDivisionByZero},
		{"div zero by zero", Div, 0, 0, 0, ErrDivisionByZero},
		{"div min by minus one", Div, min, -1, 0, ErrOverflow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn(tt.a, tt.b)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	want := []string{"sum", "sub", "mul", "div"}
	ops := All()
	if len(ops) != len(want) {
		t.Fatalf("got %d operations, want %d", len(ops), len(want))
	}
	for i, op := range ops {
		if op.Name != want[i] {
			t.Errorf("ops[%d] = %q, want %q", i, op.Name, want[i])
		}
		if op.Symbol == "" || op.Label == "" || op.Apply == nil {
			t.Errorf("operation %q is incomplete: %+v", op.Name, op)
		}
		got, ok := Lookup(op.Name)
		if !ok || got.Name != op.Name {
			t.Errorf("Lookup(%q) = %v, %v", op.Name, got, ok)
		}
	}
	if _, ok := Lookup("pow"); ok {
		t.Error("Lookup(pow) should fail")
	}

	ops[0].Name = "changed"
	if All()[0].Name != "sum" {
		t.Error("All must return a copy")
	}
}
