package utils

func Max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func Max64u(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}


func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
