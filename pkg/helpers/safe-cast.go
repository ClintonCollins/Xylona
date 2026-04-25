package helpers

import "math"

// ClampInt32FromInt converts an int to int32 while clamping overflow.
func ClampInt32FromInt(value int) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	if value < math.MinInt32 {
		return math.MinInt32
	}
	return int32(value)
}

// ClampInt32FromInt64 converts an int64 to int32 while clamping overflow.
func ClampInt32FromInt64(value int64) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	if value < math.MinInt32 {
		return math.MinInt32
	}
	return int32(value)
}

// ClampUint32FromInt64 converts an int64 to uint32 while clamping overflow.
func ClampUint32FromInt64(value int64) uint32 {
	if value <= 0 {
		return 0
	}
	if value > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(value)
}

// ClampUint32FromInt converts an int to uint32 while clamping overflow.
func ClampUint32FromInt(value int) uint32 {
	if value <= 0 {
		return 0
	}
	if value > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(value)
}

// ClampUint32FromUint64 converts a uint64 to uint32 while clamping overflow.
func ClampUint32FromUint64(value uint64) uint32 {
	if value > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(value)
}

// ClampInt64FromUint64 converts a uint64 to int64 while clamping overflow.
func ClampInt64FromUint64(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
}

// ClampUint64FromInt64 converts an int64 to uint64 while clamping overflow.
func ClampUint64FromInt64(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}
