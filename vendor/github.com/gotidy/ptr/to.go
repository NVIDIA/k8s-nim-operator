package ptr

import "time"

// To returns the value of the pointer passed in or the default value if the pointer is nil.
func To[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}

// ToInt returns the value of the int pointer passed in or int if the pointer is nil.
// ToInt returns the value of the int pointer passed in or 0 if the pointer is nil.
func ToInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// ToInt8 returns the value of the int8 pointer passed in or 0 if the pointer is nil.
func ToInt8(v *int8) int8 {
	if v == nil {
		return 0
	}
	return *v
}

// ToInt16 returns the value of the int16 pointer passed in or 0 if the pointer is nil.
func ToInt16(v *int16) int16 {
	if v == nil {
		return 0
	}
	return *v
}

// ToInt32 returns the value of the int32 pointer passed in or 0 if the pointer is nil.
func ToInt32(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}

// ToInt64 returns the value of the int64 pointer passed in or 0 if the pointer is nil.
func ToInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// ToUInt returns the value of the uint pointer passed in or 0 if the pointer is nil.
func ToUInt(v *uint) uint {
	if v == nil {
		return 0
	}
	return *v
}

// ToUInt8 returns the value of the uint8 pointer passed in or 0 if the pointer is nil.
func ToUInt8(v *uint8) uint8 {
	if v == nil {
		return 0
	}
	return *v
}

// ToUInt16 returns the value of the uint16 pointer passed in or 0 if the pointer is nil.
func ToUInt16(v *uint16) uint16 {
	if v == nil {
		return 0
	}
	return *v
}

// ToUInt32 returns the value of the uint32 pointer passed in or 0 if the pointer is nil.
func ToUInt32(v *uint32) uint32 {
	if v == nil {
		return 0
	}
	return *v
}

// ToUInt64 returns the value of the uint64 pointer passed in or 0 if the pointer is nil.
func ToUInt64(v *uint64) uint64 {
	if v == nil {
		return 0
	}
	return *v
}

// ToByte returns the value of the byte pointer passed in or 0 if the pointer is nil.
func ToByte(v *byte) byte {
	if v == nil {
		return 0
	}
	return *v
}

// ToRune returns the value of the rune pointer passed in or 0 if the pointer is nil.
func ToRune(v *rune) rune {
	if v == nil {
		return 0
	}
	return *v
}

// ToBool returns the value of the bool pointer passed in or false if the pointer is nil.
func ToBool(v *bool) bool {
	if v == nil {
		return false
	}
	return *v
}

// ToString returns the value of the string pointer passed in or empty string if the pointer is nil.
func ToString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// ToFloat32 returns the value of the float32 pointer passed in or 0 if the pointer is nil.
func ToFloat32(v *float32) float32 {
	if v == nil {
		return 0
	}
	return *v
}

// ToFloat64 returns the value of the float64 pointer passed in or 0 if the pointer is nil.
func ToFloat64(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

// ToComplex64 returns the value of the complex64 pointer passed in or 0 if the pointer is nil.
func ToComplex64(v *complex64) complex64 {
	if v == nil {
		return 0
	}
	return *v
}

// ToComplex128 returns the value of the complex128 pointer passed in or 0 if the pointer is nil.
func ToComplex128(v *complex128) complex128 {
	if v == nil {
		return 0
	}
	return *v
}

// ToDuration returns the value of the time.Duration pointer passed in or 0 if the pointer is nil.
func ToDuration(v *time.Duration) time.Duration {
	if v == nil {
		return 0
	}
	return *v
}

// ToTime returns the value of the time.Time pointer passed in or Time{} if the pointer is nil.
func ToTime(v *time.Time) time.Time {
	if v == nil {
		return time.Time{}
	}
	return *v
}
