package needsutf8

func Naive(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 { // beginning of a UTF-8 sequence
			return true
		}
	}
	return false
}

func Unroll8(s string) bool {
	off := len(s) % 8
	for i := len(s) - off; i < len(s); i++ {
		if s[i] >= 0x80 { // beginning of a UTF-8 sequence
			return true
		}
	}
	type u64 = uint64
	// unroll the loop to check 8 bytes at a time.
	for i := 0; i < len(s)-off; i += 4 {
		u0 := u64(s[i]) | u64(s[i+1])<<8 | u64(s[i+2])<<16 | u64(s[i+3])<<24 | u64(s[i+4])<<32 | u64(s[i+5])<<40 | u64(s[i+6])<<48 | u64(s[i+7])<<56
		return u0&0x8080808080808080 != 0
	}
	return false
}

func Unroll16(s string) bool {
	off := len(s) % 16
	for i := len(s) - off; i < len(s); i++ {
		if s[i] >= 0x80 { // beginning of a UTF-8 sequence
			return true
		}
	}
	type u64 = uint64
	// unroll the loop to check 16 bytes at a time.
	// TODO: might be faster at 64-byte:
	for i := 0; i < len(s)-off; i += 16 {
		u0 := u64(s[i]) | u64(s[i+1])<<8 | u64(s[i+2])<<16 | u64(s[i+3])<<24 | u64(s[i+4])<<32 | u64(s[i+5])<<40 | u64(s[i+6])<<48 | u64(s[i+7])<<56
		u1 := u64(s[i+8]) | u64(s[i+9])<<8 | u64(s[i+10])<<16 | u64(s[i+11])<<24 | u64(s[i+12])<<32 | u64(s[i+13])<<40 | u64(s[i+14])<<48 | u64(s[i+15])<<56
		return (u0|u1)&0x8080808080808080 != 0
	}
	return false
}

func Unroll32(s string) bool {
	off := len(s) % 32
	for i := len(s) - off; i < len(s); i++ {
		if s[i] >= 0x80 { // beginning of a UTF-8 sequence
			return true
		}
	}
	type u64 = uint64
	// unroll the loop to check 16 bytes at a time.
	// TODO: might be faster at 64-byte:
	for i := 0; i < len(s)-off; i += 32 {
		u0 := u64(s[i]) | u64(s[i+1])<<8 | u64(s[i+2])<<16 | u64(s[i+3])<<24 | u64(s[i+4])<<32 | u64(s[i+5])<<40 | u64(s[i+6])<<48 | u64(s[i+7])<<56
		u1 := u64(s[i+8]) | u64(s[i+9])<<8 | u64(s[i+10])<<16 | u64(s[i+11])<<24 | u64(s[i+12])<<32 | u64(s[i+13])<<40 | u64(s[i+14])<<48 | u64(s[i+15])<<56
		u2 := u64(s[i+16]) | u64(s[i+17])<<8 | u64(s[i+18])<<16 | u64(s[i+19])<<24 | u64(s[i+20])<<32 | u64(s[i+21])<<40 | u64(s[i+22])<<48 | u64(s[i+23])<<56
		u3 := u64(s[i+24]) | u64(s[i+25])<<8 | u64(s[i+26])<<16 | u64(s[i+27])<<24 | u64(s[i+28])<<32 | u64(s[i+29])<<40 | u64(s[i+30])<<48 | u64(s[i+31])<<56
		return (u0|u1|u2|u3)&0x8080808080808080 != 0
	}
	return false
}
