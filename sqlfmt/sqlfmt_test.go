package sqlfmt_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/runpod/go-tql/sqlfmt"
)

func TestSafeFormatter(t *testing.T) {
	t.Parallel()

	t.Run("bad type", func(t *testing.T) {
		t.Parallel()

		s := sqlfmt.Sprint(struct {
			X int
		}{5})
		const want = `/*<bad type struct { X int }>*/`
		if s != want {
			t.Errorf("got %q, want %q", s, want)
		}
	})

	for _, tt := range []struct {
		name  string
		input any
		want  string
	}{
		{name: "trivial", input: "hello", want: "hello"},
		{name: "quote", input: []byte("hell'o"), want: `'hell\'o'`},
		{name: "newline", input: "hello\nworld", want: "'hello\\nworld'"},
		{name: "unicode printable non-ascii", input: "🔥 and 💧", want: `utf8mb4'🔥 and 💧'`},
		{name: "special ASCII escapes", input: "\a\b\f\n\r\t\v\\", want: `'\a\b\f\n\r\t\v\\'`},
		{name: "non-printable ASCII", input: "\x00", want: `'\0'`},
		{name: "non-printable non-ASCII", input: "\x80", want: "utf8mb4'\x80'"},
		{"zero-width space", "\u200b", "utf8mb4'\u200b'"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := sqlfmt.Sprint(tt.input); got != tt.want {
				t.Errorf("got\n\t%s\nwant\n\t%s", got, tt.want)
			}
		})
	}

	for _, tt := range []struct {
		input any
		want  string
	}{
		{int8(1), "1"},
		{int16(1), "1"},
		{int32(1), "1"},
		{int64(1), "1"},
		{int(1), "1"},
		{uint8(1), "1"},
		{uint16(1), "1"},
		{uint32(1), "1"},
		{uint64(1), "1"},
		{uint(1), "1"},
		{uintptr(1), "1"},
		{float32(1), "1"},
		{float64(1), "1"},
		{true, "TRUE"},
		{false, "FALSE"},
		{nil, "NULL"},
	} {
		t.Run(fmt.Sprintf("%T", tt.input), func(t *testing.T) {
			t.Parallel()

			if sqlfmt.Sprint(tt.input) != tt.want {
				t.Errorf("got %q, want %q", sqlfmt.Sprint(tt.input), tt.want)
			}
		})
	}
}

func ExamplePrintln() {
	sqlfmt.Println("hello", "world")
	sqlfmt.Println("HI'HI", "world") // dangerous: ' could be interpreted as a string delimiter
	// Output:
	// hello world
	// 'HI\'HI' world
}

func ExamplePrint() {
	sqlfmt.Print("HI'HI\n")
	// Output:
	// 'HI\'HI\n'
}

func ExampleAppendf() {
	b := sqlfmt.Appendf(nil, "the quick brown %s jumps over the lazy %s", "'''fox'''", "dog")
	fmt.Println(string(b))
	// Output:
	// the quick brown '\'\'\'fox\'\'\'' jumps over the lazy dog
}

func ExampleSprintf() {
	s := sqlfmt.Sprintf("the quick brown %s jumps over the lazy %s", "'''fox'''", "dog")
	fmt.Println(s)
	// Output:
	// the quick brown '\'\'\'fox\'\'\'' jumps over the lazy dog
}

func ExampleFprintf() {
	sqlfmt.Fprintf(os.Stdout, "the quick brown %s jumps over the lazy %s", "'''fox'''", "dog")
	// Output:
	// the quick brown '\'\'\'fox\'\'\'' jumps over the lazy dog
}
