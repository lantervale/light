package fetch

import "context"

type referrerKey struct{}

// Referring is ctx for requests that say they come from address, in
// their Referer header: a sheet's fonts, from the sheet. "" says nothing.
func Referring(ctx context.Context, address string) context.Context {
	return context.WithValue(ctx, referrerKey{}, address)
}
