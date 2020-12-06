package handler

import (
	"github.com/jmbayu/godns"
	"github.com/jmbayu/godns/handler/alidns"
	"github.com/jmbayu/godns/handler/cloudflare"
	"github.com/jmbayu/godns/handler/dnspod"
	"github.com/jmbayu/godns/handler/dreamhost"
	"github.com/jmbayu/godns/handler/duck"
	"github.com/jmbayu/godns/handler/google"
	"github.com/jmbayu/godns/handler/he"
	"github.com/jmbayu/godns/handler/noip"
)

// IHandler is the interface for all DNS handlers
type IHandler interface {
	SetConfiguration(*godns.Settings)
	DomainLoop(domain *godns.Domain, panicChan chan<- godns.Domain)
}

// CreateHandler creates DNS handler by different providers
func CreateHandler(provider string) IHandler {
	var handler IHandler

	switch provider {
	case godns.CLOUDFLARE:
		handler = IHandler(&cloudflare.Handler{})
	case godns.DNSPOD:
		handler = IHandler(&dnspod.Handler{})
	case godns.DREAMHOST:
		handler = IHandler(&dreamhost.Handler{})
	case godns.HE:
		handler = IHandler(&he.Handler{})
	case godns.ALIDNS:
		handler = IHandler(&alidns.Handler{})
	case godns.GOOGLE:
		handler = IHandler(&google.Handler{})
	case godns.DUCK:
		handler = IHandler(&duck.Handler{})
	case godns.NOIP:
		handler = IHandler(&noip.Handler{})
	}

	return handler
}
