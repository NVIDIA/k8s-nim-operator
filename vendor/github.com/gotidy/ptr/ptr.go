package ptr

import "time"

// Of returns pointer to value.
func Of[T any](v T) *T {
	return &v
}

// Int returns pointer to int value.
func Int(v int) *int {
	return &v
}

// Int8 returns pointer to int8 value.
func Int8(v int8) *int8 {
	return &v
}

// Int16 returns pointer to int16 value.
func Int16(v int16) *int16 {
	return &v
}

// Int32 returns pointer to int32 value.
func Int32(v int32) *int32 {
	return &v
}

// Int64 returns pointer to int64 value.
func Int64(v int64) *int64 {
	return &v
}

// UInt returns pointer to uint value.
func UInt(v uint) *uint {
	return &v
}

// UInt8 returns pointer to uint8 value.
func UInt8(v uint8) *uint8 {
	return &v
}

// UInt16 returns pointer to uint16 value.
func UInt16(v uint16) *uint16 {
	return &v
}

// UInt32 returns pointer to uint32 value.
func UInt32(v uint32) *uint32 {
	return &v
}

// UInt64 returns pointer to uint64 value.
func UInt64(v uint64) *uint64 {
	return &v
}

// Byte returns pointer to byte value.
func Byte(v byte) *byte {
	return &v
}

// Rune returns pointer to rune value.
func Rune(v rune) *rune {
	return &v
}

// Bool returns pointer to bool value.
func Bool(v bool) *bool {
	return &v
}

// String returns pointer to string value.
func String(v string) *string {
	return &v
}

// Float32 returns pointer to float32 value.
func Float32(v float32) *float32 {
	return &v
}

// Float64 returns pointer to float64 value.
func Float64(v float64) *float64 {
	return &v
}

// Complex64 returns pointer to complex64 value.
func Complex64(v complex64) *complex64 {
	return &v
}

// Complex128 returns pointer to complex128 value.
func Complex128(v complex128) *complex128 {
	return &v
}

// Duration returns pointer to time.Duration value.
func Duration(v time.Duration) *time.Duration {
	return &v
}

// Time returns pointer to time.Time value.
func Time(v time.Time) *time.Time {
	return &v
}
