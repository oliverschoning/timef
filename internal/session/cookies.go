package session

import (
	"fmt"
	"net/http"

	"github.com/browserutils/kooky"
	_ "github.com/browserutils/kooky/browser/all"
)

const cookieDomainSuffix = "poweroffice.net"

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
