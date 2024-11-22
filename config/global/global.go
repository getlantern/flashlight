// This package breaks out some global config handling to where it can be used externally without dependence on all flashlight config.
package globalconfig

import (
	"errors"
)

const FeatureReplica = "replica"

// This is equivalent config.Config without any of the extra functionality.
type Raw any

func UnmarshalFeatureOptions(cfg Raw, feature string, opts FeatureOptions) error {
	// It's possible to do this with recover and runtime type assertion checking, but we actually want to screen for nils too.
	foAny, exists := cfg.(map[any]any)["featureoptions"]
	if !exists {
		return ErrFeatureOptionAbsent
	}
	fAny, exists := foAny.(map[any]any)[feature]
	if !exists {
		return ErrFeatureOptionAbsent
	}
	mAnyKeys := fAny.(map[any]any)
	mStrKeys := make(map[string]any, len(mAnyKeys))
	for k, v := range mAnyKeys {
		mStrKeys[k.(string)] = v
	}
	return opts.FromMap(mStrKeys)
}

// FeatureOptions is an interface implemented by all feature options
type FeatureOptions interface {
	FromMap(map[string]interface{}) error
}

var ErrFeatureOptionAbsent = errors.New("feature option is absent")
