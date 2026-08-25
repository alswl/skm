// Package timing is a generic stopwatch helper with no domain dependencies.
package timing

import "time"

// Measurement records the elapsed time for a named operation.
type Measurement struct {
	Name    string        `json:"name"`
	Elapsed time.Duration `json:"elapsed_ns"`
}

// Measure times fn over ops repetitions, returning the average elapsed time.
func Measure(name string, ops int, fn func()) Measurement {
	start := time.Now()
	for i := 0; i < ops; i++ {
		fn()
	}
	return Measurement{Name: name, Elapsed: time.Duration(int64(time.Since(start)) / int64(ops))}
}
