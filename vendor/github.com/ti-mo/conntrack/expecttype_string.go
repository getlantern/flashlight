// Code generated by "stringer -type=expectType"; DO NOT EDIT.

package conntrack

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[ctaExpectUnspec-0]
	_ = x[ctaExpectMaster-1]
	_ = x[ctaExpectTuple-2]
	_ = x[ctaExpectMask-3]
	_ = x[ctaExpectTimeout-4]
	_ = x[ctaExpectID-5]
	_ = x[ctaExpectHelpName-6]
	_ = x[ctaExpectZone-7]
	_ = x[ctaExpectFlags-8]
	_ = x[ctaExpectClass-9]
	_ = x[ctaExpectNAT-10]
	_ = x[ctaExpectFN-11]
}

const _expectType_name = "ctaExpectUnspecctaExpectMasterctaExpectTuplectaExpectMaskctaExpectTimeoutctaExpectIDctaExpectHelpNamectaExpectZonectaExpectFlagsctaExpectClassctaExpectNATctaExpectFN"

var _expectType_index = [...]uint8{0, 15, 30, 44, 57, 73, 84, 101, 114, 128, 142, 154, 165}

func (i expectType) String() string {
	if i >= expectType(len(_expectType_index)-1) {
		return "expectType(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _expectType_name[_expectType_index[i]:_expectType_index[i+1]]
}
