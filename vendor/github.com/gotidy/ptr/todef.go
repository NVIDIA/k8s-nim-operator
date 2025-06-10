package ptr

import "time"

// ToDef returns the value of the int pointer passed in or default value if the pointer is nil.
func ToDef[T any](v *T, def T) T {
	if v == nil {
		return def
	}
	return *v
}

// ToIntDef returns the value of the int pointer passed in or default value if the pointer is nil.
func ToIntDef(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}

// ToInt8Def returns the value of the int8 pointer passed in or default value if the pointer is nil.
func ToInt8Def(v *int8, def int8) int8 {
	if v == nil {
		return def
	}
	return *v
}

// ToInt16Def returns the value of the int16 pointer passed in or default value if the pointer is nil.
func ToInt16Def(v *int16, def int16) int16 {
	if v == nil {
		return def
	}
	return *v
}

// ToInt32Def returns the value of the int32 pointer passed in or default value if the pointer is nil.
func ToInt32Def(v *int32, def int32) int32 {
	if v == nil {
		return def
	}
	return *v
}

// ToInt64Def returns the value of the int64 pointer passed in or default value if the pointer is nil.
func ToInt64Def(v *int64, def int64) int64 {
	if v == nil {
		return def
	}
	return *v
}

// ToUIntDef returns the value of the uint pointer passed in or default value if the pointer is nil.
func ToUIntDef(v *uint, def uint) uint {
	if v == nil {
		return def
	}
	return *v
}

// ToUInt8Def returns the value of the uint8 pointer passed in or default value if the pointer is nil.
func ToUInt8Def(v *uint8, def uint8) uint8 {
	if v == nil {
		return def
	}
	return *v
}

// ToUInt16Def returns the value of the uint16 pointer passed in or default value if the pointer is nil.
func ToUInt16Def(v *uint16, def uint16) uint16 {
	if v == nil {
		return def
	}
	return *v
}

// ToUInt32Def returns the value of the uint32 pointer passed in or default value if the pointer is nil.
func ToUInt32Def(v *uint32, def uint32) uint32 {
	if v == nil {
		return def
	}
	return *v
}

// ToUInt64Def returns the value of the uint64 pointer passed in or default value if the pointer is nil.
func ToUInt64Def(v *uint64, def uint64) uint64 {
	if v == nil {
		return def
	}
	return *v
}

// ToByteDef returns the value of the byte pointer passed in or default value if the pointer is nil.
func ToByteDef(v *byte, def byte) byte {
	if v == nil {
		return def
	}
	return *v
}

// ToRuneDef returns the value of the rune pointer passed in or default value if the pointer is nil.
func ToRuneDef(v *rune, def rune) rune {
	if v == nil {
		return def
	}
	return *v
}

// ToBoolDef returns the value of the bool pointer passed in or default value if the pointer is nil.
func ToBoolDef(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// ToStringDef returns the value of the string pointer passed in or default value if the pointer is nil.
func ToStringDef(v *string, def string) string {
	if v == nil {
		return def
	}
	return *v
}

// ToFloat32Def returns the value of the float32 pointer passed in or default value if the pointer is nil.
func ToFloat32Def(v *float32, def float32) float32 {
	if v == nil {
		return def
	}
	return *v
}

// ToFloat64Def returns the value of the float64 pointer passed in or default value if the pointer is nil.
func ToFloat64Def(v *float64, def float64) float64 {
	if v == nil {
		return def
	}
	return *v
}

// ToComplex64Def returns the value of the complex64 pointer passed in or default value if the pointer is nil.
func ToComplex64Def(v *complex64, def complex64) complex64 {
	if v == nil {
		return def
	}
	return *v
}

// ToComplex128Def returns the value of the complex128 pointer passed in or default value if the pointer is nil.
func ToComplex128Def(v *complex128, def complex128) complex128 {
	if v == nil {
		return def
	}
	return *v
}

// ToDurationDef returns the value of the time.Duration pointer passed in or default value if the pointer is nil.
func ToDurationDef(v *time.Duration, def time.Duration) time.Duration {
	if v == nil {
		return def
	}
	return *v
}

// ToTimeDef returns the value of the time.Time pointer passed in or default value if the pointer is nil.
func ToTimeDef(v *time.Time, def time.Time) time.Time {
	if v == nil {
		return def
	}
	return *v
}
