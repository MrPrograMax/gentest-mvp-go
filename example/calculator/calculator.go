// Package calculator is a simple example package used to demonstrate testgen.
package calculator

import (
	"errors"
	"strings"
	"time"
)

// Add returns the sum of a and b.
func Add(a, b int) int {
	return a + b
}

// Divide returns a/b or an error if b is zero.
func Divide(a, b float64) (float64, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}

// Greet returns a personalised greeting or an error for empty names.
func Greet(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("name must not be empty")
	}
	return "Hello, " + name + "!", nil
}

// Sum returns the sum of all provided integers.
func Sum(nums ...int) int {
	total := 0
	for _, n := range nums {
		total += n
	}
	return total
}

// Since returns how long ago t was relative to now.
func Since(t time.Time) time.Duration {
	return time.Since(t)
}

// Filter returns only the elements of items for which keep returns true.
func Filter(items []string, keep func(string) bool) []string {
	var out []string
	for _, item := range items {
		if keep(item) {
			out = append(out, item)
		}
	}
	return out
}
