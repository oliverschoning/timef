package session

import (
	"fmt"
	"net/http"

	"github.com/browserutils/kooky"
	_ "github.com/browserutils/kooky/browser/all"
)

const cookieDomainSuffix = "poweroffice.net"

// goSessionCookieName is the BFF session cookie set on go.poweroffice.net after
// a successful browser login. Its presence on disk = login flushed.
const goSessionCookieName = "__Host-bff"
const goSessionDomain = "go.poweroffice.net"

// HasGoSession reports whether the BFF session cookie for go.poweroffice.net is
// present (and valid) in the on-disk browser cookie store. Browsers buffer
// freshly-set cookies in memory and flush to disk periodically, so right after
// login this returns false until the flush lands.
func HasGoSession() bool {
	cookies := kooky.AllCookies(
		kooky.Valid,
		kooky.Name(goSessionCookieName),
		kooky.Domain(goSessionDomain),
	)
	for _, c := range cookies {
		if c.Value != "" {
			return true
		}
	}
	return false
}

func LoadCookies() ([]*http.Cookie, error) {
	cookies := kooky.AllCookies(
		kooky.Valid,
		kooky.DomainHasSuffix(cookieDomainSuffix),
	)
	if len(cookies) == 0 {
		return nil, fmt.Errorf("no cookies for %s — log in via your default browser first", cookieDomainSuffix)
	}

	out := make([]*http.Cookie, 0, len(cookies))
	for _, c := range cookies {
		out = append(out, &c.Cookie)
	}
	return out, nil
}
