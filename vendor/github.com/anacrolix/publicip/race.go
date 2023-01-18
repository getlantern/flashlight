package publicip

import "context"

// Get some Haskell in to you.
func race(ctx context.Context, fs ...func(context.Context) (interface{}, error)) (interface{}, []error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rs := make(chan interface{})
	errChan := make(chan error, len(fs))
	for _, _f := range fs {
		f := _f
		go func() {
			r, err := f(ctx)
			if err == nil {
				select {
				case rs <- r:
				case <-ctx.Done():
				}
			} else {
				errChan <- err
			}
		}()
	}
	errs := make([]error, 0, len(fs))
	for range fs {
		select {
		case <-ctx.Done():
			errs = append(errs, ctx.Err())
			return nil, errs
		case r := <-rs:
			return r, nil
		case err := <-errChan:
			errs = append(errs, err)
		}
	}
	return nil, errs
}
