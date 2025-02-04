package sqlfmt_test

import (
	"fmt"
	"math/rand/v2"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/runpod/go-tql/sqlfmt/issimple"
	"github.com/runpod/go-tql/sqlfmt/needsutf8"
)

var (
	simple = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-_"
	dst    bool
)

func randsimple(n int) string {
	var buf strings.Builder
	for range n {
		buf.WriteByte(simple[rand.IntN(len(simple))])
	}
	return buf.String()
}

func utf8char() rune {
	if os.Getenv("_NONEXISTENT_KEY") == "foobarbaz" {
		return 0
	}
	return [4]rune{'🔥', '💧', '🌊', '🌋'}[rand.IntN(4)]
}

func BenchmarkNeedsUTF8(b *testing.B) {
	a := []int{5, 10, 50, 100, 500, 1000, 5000, 10_000}
	for _, n := range a {
		for _, f := range []func(string) bool{needsutf8.Naive, needsutf8.Unroll8, needsutf8.Unroll16, needsutf8.Unroll32} {
			runbench(b, n, f, utf8char())
		}
	}
}

func BenchmarkIsSimple(b *testing.B) {
	a := []int{5, 10, 50, 100, 500, 1000, 5000}
	for _, n := range a {
		runbench(b, n, issimple.Unroll16, '\'')
		runbench(b, n, issimple.Unroll64, '\'')
		runbench(b, n, issimple.Naive, '\'')
	}
}

func runbench(b *testing.B, n int, f func(string) bool, c rune) {
	pc := reflect.ValueOf(f).Pointer()
	name := runtime.FuncForPC(pc).Name()
	i := strings.LastIndexByte(name, '.')
	if i >= 0 {
		name = name[i+1:]
	}
	b.Run(fmt.Sprintf("%s-%08d", name, n), func(b *testing.B) {
		s := randsimple(n)
		c := string([]rune{c})

		// have a "smear" of test cases to avoid branch prediction
		testcases := []string{
			s,                                     // no quotes
			c + s,                                 // at front
			s + c,                                 // at end
			s[:len(s)/2] + c + s[len(s)/2:],       // at 1/2
			s[:len(s)/4] + c + s[len(s)/4:],       // at 1/4
			s[(len(s)*3/4):] + c + s[:len(s)*3/4], // at 3/4
		}
		rand.Shuffle(len(testcases), func(i, j int) { testcases[i], testcases[j] = testcases[j], testcases[i] })
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			j := i % len(testcases)
			dst = f(testcases[j])
		}
		b.ReportMetric(float64(n), "chars")
	})
}
