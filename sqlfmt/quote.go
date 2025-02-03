package sqlfmt

import (
	"github.com/runpod/go-tql/sqlfmt/issimple"
	"github.com/runpod/go-tql/sqlfmt/needsutf8"
)

// quote.go contains the implementation of the Quote function, which is used to quote strings in SQL.

// SQL-quote a string. This means that they are surrounded by single quotes and any special characters are escaped.
//   - the ' (single quote) character is escaped as two single quotes.
//   - non-printable ASCII is escaped as \x followed by two hex digits.
//   - non-printable non-ASCII is escaped as \u followed by four hex digits.
//   - the usual ANSI escapable characters are escaped as \a, \b, \f, \n, \r, \t, \v, and \\.
//   - all other characters are left as-is.
func Quote(scratch []byte, s string) []byte {
	scratch = scratch[:0]
	if NeedsUTF8(s) {
		const utf8off = len("utf8mb4") + len("''") + 5 //  utf8mb4, two quotes, and at least 4 characters for the utf8mb4 string.
		if cap(scratch) < len(s)+utf8off {
			scratch = make([]byte, 0, len(s)+utf8off)
		}
		scratch = append(scratch, "utf8mb4"...) // mark the string as utf8mb4. "regular" utf8 strings in mysql are an accursed variable-length encoding that only supports 3 bytes per character.
	} else if cap(scratch) < len(s)+3 { // we need at minimum 2 more bytes for the quotes, and we assume one more byte for whatever needs escaping.
		scratch = make([]byte, 0, len(s)+3)
	}
	scratch = append(scratch, '\'') // opening quote
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\a':
			scratch = append(scratch, '\\', 'a')
		case '\b':
			scratch = append(scratch, '\\', 'b')
		case '\f':
			scratch = append(scratch, '\\', 'f')
		case '\n':
			scratch = append(scratch, '\\', 'n')
		case '\r':
			scratch = append(scratch, '\\', 'r')
		case '\t':
			scratch = append(scratch, '\\', 't')
		case '\v':
			scratch = append(scratch, '\\', 'v')
		case '\\':
			scratch = append(scratch, '\\', '\\')
		case '\'':
			scratch = append(scratch, '\\', '\'')
		case 0:
			scratch = append(scratch, '\\', '0')
		case 0x1a:
			scratch = append(scratch, '\\', 'Z')
		case '%':
			scratch = append(scratch, '\\', '%')
		case '_':
			scratch = append(scratch, '\\', '_')
		default:
			scratch = append(scratch, s[i])
		}
	}

	scratch = append(scratch, '\'') // closing quote
	return scratch
}

// IsSimple checks if a string is "simple" (ASCII-only and does not contain any of the special characters that need to be escaped).
// See the package documentation for details on special characters.
func IsSimple(s string) bool {
	return issimple.Unroll16(s) // unroll16 is the fastest on AMD64 for almost all string sizes.
}

func NeedsUTF8(s string) bool {
	// All versions are roughly equivalent in speed for small strings (< 10 bytes) but the unrolling gets much, much, MUCH faster for larger strings.
	// On AMD64, for strings of 10000 characters with a single non-ASCII character inserted at a random index:
	// naive: 1211ns/op
	// unroll8: 3.9ns/op     x310.51
	// unroll16: 4.920ns/op  x246.13
	// unroll32: 14.12ns/op  x85.68
	return needsutf8.Unroll8(s)
}

// As Quote(), but omit the quoting if the string is "simple"
// (only contains ASCII letters, digits, dots, dashes, and underscores).
func QuoteIfNeeded(scratch []byte, s string) []byte {
	// check if the string is simple. if not, quote it.
	if IsSimple(s) {
		return append(scratch, ("'" + s + "'")...)
	}

	return Quote(scratch, s)
}
