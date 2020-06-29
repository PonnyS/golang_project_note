package internal

const (
	// 整型的bit数
	bitsize       = 32 << (^uint(0) >> 63)
	// 最大整型 >> 1，方便之后的“power”操作
	maxintHeadBit = 1 << (bitsize - 2)
)

func IsPowerOfTwo(n int) bool {
	return n&(n-1) == 0
}

func CeilToPowerOfTwo(n int) int {
	if n&maxintHeadBit != 0 && n > maxintHeadBit {
		panic("argument is too large")
	}
	if n <= 2 {
		return 2
	}
	n--
	n = fillBits(n)
	n++
	return n
}

func FloorToPowerOfTwo(n int) int {
	if n <= 2 {
		return 2
	}
	n = fillBits(n)
	n >>= 1
	n++
	return n
}

// 填充n之后的位，例如 10(2) -> 11(3)
func fillBits(n int) int {
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	return n
}
