package alidns

import (
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/jmbayu/godns"
)

// Handler struct
type Handler struct {
	Configuration *godns.Settings
}

// SetConfiguration pass dns settings and store it to handler instance
func (handler *Handler) SetConfiguration(conf *godns.Settings) {
	handler.Configuration = conf
}

// DomainLoop the main logic loop
func (handler *Handler) DomainLoop(domain *godns.Domain, panicChan chan<- godns.Domain) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("Recovered in %v: %v\n", err, debug.Stack())
			panicChan <- *domain
		}
	}()

	looping := false
	aliDNS := NewAliDNS(handler.Configuration.Email, handler.Configuration.Password)

	for {
		if looping {
			// Sleep with interval
			log.Printf("Going to sleep, will start next checking in %d seconds...\r\n", handler.Configuration.Interval)
			time.Sleep(time.Second * time.Duration(handler.Configuration.Interval))
		}

		looping = true
		currentIP, err := godns.GetCurrentIP(handler.Configuration)

		if err != nil {
			log.Println("Failed to get current IP:", err)
			continue
		}
		log.Println("currentIP is:", currentIP)
		for _, subDomain := range domain.SubDomains {
			hostname := subDomain + "." + domain.DomainName
			lastIP, err := godns.ResolveDNS(hostname, handler.Configuration.Resolver, handler.Configuration.IPType)
			if err != nil {
				log.Println(err)
				continue
			}
			//check against currently known IP, if no change, skip update
			if currentIP == lastIP {
				log.Printf("IP is the same as cached one. Skip update.\n")
			} else {
				lastIP = currentIP

				log.Printf("%s.%s Start to update record IP...\n", subDomain, domain.DomainName)
				records := aliDNS.GetDomainRecords(domain.DomainName, subDomain)
				if records == nil || len(records) == 0 {
					log.Printf("Cannot get subdomain %s from AliDNS.\r\n", subDomain)
					continue
				}

				records[0].Value = currentIP
				if err := aliDNS.UpdateDomainRecord(records[0]); err != nil {
					log.Printf("Failed to update IP for subdomain:%s\r\n", subDomain)
					continue
				} else {
					log.Printf("IP updated for subdomain:%s\r\n", subDomain)
				}

				// Send notification
				if err := godns.SendNotify(handler.Configuration, fmt.Sprintf("%s.%s", subDomain, domain.DomainName), currentIP); err != nil {
					log.Printf("Failed to send notification")
				}
			}
		}
	}

}
