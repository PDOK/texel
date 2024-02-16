package mathhelp

func BetweenInc(f, p, q int64) bool {
	if p <= q {
		return p <= f && f <= q
	}
	return q <= f && f <= p
}

func Pow2(n uint) uint {
	return 1 << n
}

func Bool2int(b bool) int {
	if b {
		return 1
	}
	return 0
}
