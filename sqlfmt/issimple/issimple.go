package issimple

func chk(b byte) uint64 {
	//   - The following code points must be escaped:
	//     - ' (single quote) -> \'
	//     - \ (backslash) -> \\
	//     - \a (bell), \b (backspace), \f (form feed), \n (newline), \r (carriage return), \t (tab), \v (vertical tab) -> \a, \b, \f, \n, \r, \t, \v
	//     - \x00 (null byte) -> \0
	//	   - \x1a (substitute) -> \Z
	//     - \x (non-printable ASCII) -> \x followed by two hex digits.
	//     - % (percent) -> \%
	//     - _ (underscore) -> \_
	//   - if the string contains non-ASCII characters, we mark it as a utf8 string: utf8mb4'⭐ and 🌟'
	if b < 0x20 || b > 0x7E { // outside of printable ASCII range. not simple by definition.
		return 0
	}

	switch b {
	case '\'', '\\', '\a', '\b', '\f', '\n', '\r', '\t', '\v', 0, 0x1a, '%', '_':
		return 0
	default:
		return 1
	}
}

func Naive(s string) bool {
	for i := 0; i < len(s); i++ {
		if chk(s[i]) == 0 {
			return false
		}
	}
	return true
}

// IsSimple checks if a string is "simple" (ASCII-only and does not contain any of the special characters that need to be escaped).
// See the package documentation for details on special characters.
func Unroll16(s string) bool {
	off := len(s) & 15
	for i := len(s) - off; i < len(s); i++ {
		if chk(s[i]) == 0 { // not simple
			return false
		}
	}

	if len(s) == off {
		return true
	}

	s = s[off:]

	// unroll the loop to check 16 bytes at a time.
	// this would be faster with SIMD: I should do that later.
	// it may also be faster with a more generous unroll: AMD64 can fetch 64 bytes at a time, ARM64 can fetch 128 bytes at a time.
	for i := range s {
		_ = s[i+15]
		if chk(s[i])|chk(s[i+1])<<1|chk(s[i+2])<<2|chk(s[i+3])<<3|chk(s[i+4])<<4|chk(s[i+5])<<5|chk(s[i+6])<<6|chk(s[i+7])<<7|chk(s[i+8])<<8|chk(s[i+9])<<9|chk(s[i+10])<<10|chk(s[i+11])<<11|chk(s[i+12])<<12|chk(s[i+13])<<13|chk(s[i+14])<<14|chk(s[i+15])<<15 != 0 {
			return false
		}
	}
	return true
}

func Unroll64(s string) bool {
	off := len(s) & 63
	if !Unroll16(s[:off]) {
		return false
	}
	if len(s) == off { // all done
		return true
	}
	s = s[off:]
	for i := 0; i < len(s); i += 64 {
		_ = s[i+63]
		n := chk(s[i])<<0 | chk(s[i+1])<<1 | chk(s[i+2])<<2 | chk(s[i+3])<<3 |
			chk(s[i+4])<<4 | chk(s[i+5])<<5 | chk(s[i+6])<<6 | chk(s[i+7])<<7 |
			chk(s[i+8])<<8 | chk(s[i+9])<<9 | chk(s[i+10])<<10 | chk(s[i+11])<<11 |
			chk(s[i+12])<<12 | chk(s[i+13])<<13 | chk(s[i+14])<<14 | chk(s[i+15])<<15 |
			chk(s[i+16])<<16 | chk(s[i+17])<<17 | chk(s[i+18])<<18 | chk(s[i+19])<<19 |
			chk(s[i+20])<<20 | chk(s[i+21])<<21 | chk(s[i+22])<<22 | chk(s[i+23])<<23 |
			chk(s[i+24])<<24 | chk(s[i+25])<<25 | chk(s[i+26])<<26 | chk(s[i+27])<<27 |
			chk(s[i+28])<<28 | chk(s[i+29])<<29 | chk(s[i+30])<<30 | chk(s[i+31])<<31 |
			chk(s[i+32])<<32 | chk(s[i+33])<<33 | chk(s[i+34])<<34 | chk(s[i+35])<<35 |
			chk(s[i+36])<<36 | chk(s[i+37])<<37 | chk(s[i+38])<<38 | chk(s[i+39])<<39 |
			chk(s[i+40])<<40 | chk(s[i+41])<<41 | chk(s[i+42])<<42 | chk(s[i+43])<<43 |
			chk(s[i+44])<<44 | chk(s[i+45])<<45 | chk(s[i+46])<<46 | chk(s[i+47])<<47 |
			chk(s[i+48])<<48 | chk(s[i+49])<<49 | chk(s[i+50])<<50 | chk(s[i+51])<<51 |
			chk(s[i+52])<<52 | chk(s[i+53])<<53 | chk(s[i+54])<<54 | chk(s[i+55])<<55 |
			chk(s[i+56])<<56 | chk(s[i+57])<<57 | chk(s[i+58])<<58 | chk(s[i+59])<<59 |
			chk(s[i+60])<<60 | chk(s[i+61])<<61 | chk(s[i+62])<<62 | chk(s[i+63])<<63
		if n == 0 {
			return false
		}

	}
	return true
}
