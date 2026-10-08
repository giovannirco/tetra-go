// Package calc holds the arithmetic operations exposed by the API.
//
// Every operation works on int64 and reports overflow instead of wrapping,
// so a result the API returns is always the mathematically correct one.
// To add an operation, append it to the registry below; the HTTP routes,
// the discovery endpoint and the web UI all pick it up from here.
package calc

import (
	"errors"
	"math"
)

var (
	// ErrDivisionByZero is returned by Div when the divisor is zero.
	ErrDivisionByZero = errors.New("division by zero")
	// ErrOverflow is returned when the result does not fit in an int64.
	ErrOverflow = errors.New("result overflows a 64-bit integer")
)

// Operation is one arithmetic operation served at /api/<Name>.
type Operation struct {
	Name   string // URL segment, e.g. "sum"
	Symbol string // display symbol, e.g. "+"
	Label  string // human name, e.g. "Addition"
	Apply  func(a, b int64) (int64, error)
}

// registry is the single list of supported operations, in display order.
var registry = []Operation{
	{Name: "sum", Symbol: "+", Label: "Addition", Apply: Add},
	{Name: "sub", Symbol: "−", Label: "Subtraction", Apply: Sub},
	{Name: "mul", Symbol: "×", Label: "Multiplication", Apply: Mul},
	{Name: "div", Symbol: "÷", Label: "Division", Apply: Div},
}

// All returns a copy of the supported operations in display order.
func All() []Operation {
	return append([]Operation(nil), registry...)
}

// Lookup returns the operation with the given name.
func Lookup(name string) (Operation, bool) {
	for _, op := range registry {
		if op.Name == name {
			return op, true
		}
	}
	return Operation{}, false
}

// Add returns a + b.
func Add(a, b int64) (int64, error) {
	r := a + b
	if (b > 0 && r < a) || (b < 0 && r > a) {
		return 0, ErrOverflow
	}
	return r, nil
}

// Sub returns a - b.
func Sub(a, b int64) (int64, error) {
	r := a - b
	if (b > 0 && r > a) || (b < 0 && r < a) {
		return 0, ErrOverflow
	}
	return r, nil
}

// Mul returns a * b.
func Mul(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if (a == -1 && b == math.MinInt64) || (b == -1 && a == math.MinInt64) {
		return 0, ErrOverflow
	}
	r := a * b
	if r/b != a {
		return 0, ErrOverflow
	}
	return r, nil
}

// Div returns a / b, truncated toward zero (7/2 = 3, -7/2 = -3).
func Div(a, b int64) (int64, error) {
	if b == 0 {
		return 0, ErrDivisionByZero
	}
	if a == math.MinInt64 && b == -1 {
		return 0, ErrOverflow
	}
	return a / b, nil
}
