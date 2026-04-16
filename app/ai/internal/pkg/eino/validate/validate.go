package validate

import "context"

type Options struct {
	IDs []int
}

type Option struct {
	apply             func(opt *Options)
	implSpecificOptFn any
}

// GetCommonOptions extract validator Options from Option list, optionally providing a base Options with default values.
func GetCommonOptions(base *Options, opts ...Option) *Options {
	if base == nil {
		base = &Options{}
	}

	for i := range opts {
		opt := opts[i]
		if opt.apply != nil {
			opt.apply(base)
		}
	}

	return base
}

// WrapImplSpecificOptFn 组件实现方基于此方法，封装自己的Option函数：func WithXXX(xxx string) Option{}
func WrapImplSpecificOptFn[T any](optFn func(*T)) Option {
	return Option{
		implSpecificOptFn: optFn,
	}
}

// GetImplSpecificOptions provides tool author the ability to extract their own custom options from the unified Option type.
// T: the type of the impl specific options struct.
// This function should be used within the tool implementation's InvokableRun or StreamableRun functions.
// It is recommended to provide a base T as the first argument, within which the tool author can provide default values for the impl specific options.
func GetImplSpecificOptions[T any](base *T, opts ...Option) *T {
	if base == nil {
		base = new(T)
	}

	for i := range opts {
		opt := opts[i]
		if opt.implSpecificOptFn != nil {
			optFn, ok := opt.implSpecificOptFn.(func(*T))
			if ok {
				optFn(base)
			}
		}
	}

	return base
}

// WithIDs sets the IDs of the data objects to validate.
func WithIDs(ids ...int) Option {
	return Option{
		apply: func(opt *Options) {
			opt.IDs = ids
		},
	}
}

type Validator interface {
	validate(ctx context.Context, opts ...Option) error
}
