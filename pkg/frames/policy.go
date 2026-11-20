package frames

import "strings"

// Allows reports whether the document of origin an iframe shows may use
// a feature of the Permissions Policy whose default allowlist is 'self',
// by the iframe's allow attribute: self is the origin of the document
// the iframe is in, and src that of the address its src names. A feature
// the attribute does not name is the document's only when it is of
// self's origin; one it names without an allowlist is src's ('src'),
// and an opaque origin ("null") matches only '*'. That the document
// round it may use the feature is the caller's to check.
func Allows(allow, feature, self, src, origin string) bool {
	for _, directive := range strings.Split(allow, ";") {
		fields := strings.Fields(directive)
		if len(fields) == 0 || !strings.EqualFold(fields[0], feature) {
			continue
		}
		list := fields[1:]
		if len(list) == 0 {
			list = []string{"'src'"}
		}
		for _, item := range list {
			switch strings.ToLower(item) {
			case "*":
				return true
			case "'self'":
				if origin == self && origin != opaque {
					return true
				}
			case "'src'":
				if origin == src && origin != opaque {
					return true
				}
			case "'none'":
			default:
				if strings.EqualFold(strings.TrimSuffix(item, "/"), origin) && origin != opaque {
					return true
				}
			}
		}
		return false
	}
	return origin == self && origin != opaque
}
