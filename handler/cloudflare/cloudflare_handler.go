package cloudflare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jmbayu/godns"
)

// Handler struct definition
type Handler struct {
	Configuration *godns.Settings
	API           string
}

// DNSRecordResponse struct
type DNSRecordResponse struct {
	Records []DNSRecord `json:"result"`
	Success bool        `json:"success"`
}

// DNSRecordUpdateResponse struct
type DNSRecordUpdateResponse struct {
	Record  DNSRecord `json:"result"`
	Success bool      `json:"success"`
}

// DNSRecord for Cloudflare API
type DNSRecord struct {
	ID      string `json:"id"`
	IP      string `json:"content"`
	Name    string `json:"name"`
	Proxied bool   `json:"proxied"`
	Type    string `json:"type"`
	ZoneID  string `json:"zone_id"`
	TTL     int32  `json:"ttl"`
}

// SetIP updates DNSRecord.IP
func (r *DNSRecord) SetIP(ip string) {
	r.IP = ip
}

// ZoneResponse is a wrapper for Zones
type ZoneResponse struct {
	Zones   []Zone `json:"result"`
	Success bool   `json:"success"`
}

// Zone object with id and name
type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SetConfiguration pass dns settings and store it to handler instance
func (handler *Handler) SetConfiguration(conf *godns.Settings) {
	handler.Configuration = conf
	handler.API = "https://api.cloudflare.com/client/v4"
}

// DomainLoop the main logic loop
func (handler *Handler) DomainLoop(domain *godns.Domain, panicChan chan<- godns.Domain) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("Recovered in %v: %v\n", err, debug.Stack())
			panicChan <- *domain
		}
	}()

	var lastIP string
	looping := false
	for {
		if looping {
			// Sleep with interval
			log.Printf("Going to sleep, will start next checking in %d seconds...\r\n", handler.Configuration.Interval)
			time.Sleep(time.Second * time.Duration(handler.Configuration.Interval))
		}
		looping = true

		currentIP, err := godns.GetCurrentIP(handler.Configuration)
		if err != nil {
			log.Println("Error in GetCurrentIP:", err)
			continue
		}
		log.Println("Current IP is:", currentIP)
		//check against locally cached IP, if no change, skip update
		if currentIP == lastIP {
			log.Printf("IP is the same as cached one. Skip update.\n")
		} else {
			log.Println("Checking IP for domain", domain.DomainName)
			zoneID := handler.getZone(domain.DomainName)
			if zoneID != "" {
				records := handler.getDNSRecords(zoneID)

				// update records
				for _, rec := range records {
					if !recordTracked(domain, &rec) {
						log.Println("Skiping record:", rec.Name)
						continue
					}
					if rec.IP != currentIP {
						log.Printf("IP mismatch: Current(%+v) vs Cloudflare(%+v)\r\n", currentIP, rec.IP)
						lastIP = handler.updateRecord(rec, currentIP)

						// Send notification
						if err := godns.SendNotify(handler.Configuration, rec.Name, currentIP); err != nil {
							log.Println("Failed to send notification")
						}
					} else {
						log.Printf("Record OK: %+v - %+v\r\n", rec.Name, rec.IP)
					}
				}
			} else {
				log.Println("Failed to find zone for domain:", domain.DomainName)
			}
		}
	}
}

// Check if record is present in domain conf
func recordTracked(domain *godns.Domain, record *DNSRecord) bool {
	for _, subDomain := range domain.SubDomains {
		sd := fmt.Sprintf("%s.%s", subDomain, domain.DomainName)
		if record.Name == sd {
			return true
		}
	}

	return false
}

// Create a new request with auth in place and optional proxy
func (handler *Handler) newRequest(method, url string, body io.Reader) (*http.Request, *http.Client) {
	client := godns.GetHttpClient(handler.Configuration, handler.Configuration.UseProxy)
	if client == nil {
		log.Println("cannot create HTTP client")
	}

	req, _ := http.NewRequest(method, handler.API+url, body)
	req.Header.Set("Content-Type", "application/json")

	if handler.Configuration.Email != "" && handler.Configuration.Password != "" {
		req.Header.Set("X-Auth-Email", handler.Configuration.Email)
		req.Header.Set("X-Auth-Key", handler.Configuration.Password)
	} else if handler.Configuration.LoginToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", handler.Configuration.LoginToken))
	}

	return req, client
}

// Find the correct zone via domain name
func (handler *Handler) getZone(domain string) string {

	var z ZoneResponse

	req, client := handler.newRequest("GET", fmt.Sprintf("/zones?name=%s", domain), nil)
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Request error:", err.Error())
		return ""
	}

	body, _ := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &z)
	if err != nil {
		log.Printf("Decoder error: %+v\n", err)
		log.Printf("Response body: %+v\n", string(body))
		return ""
	}
	if z.Success != true {
		log.Printf("Response failed: %+v\n", string(body))
		return ""
	}

	for _, zone := range z.Zones {
		if zone.Name == domain {
			return zone.ID
		}
	}
	return ""
}

// Get all DNS A records for a zone
func (handler *Handler) getDNSRecords(zoneID string) []DNSRecord {

	var empty []DNSRecord
	var r DNSRecordResponse
	var recordType string

	if handler.Configuration.IPType == "" || strings.ToUpper(handler.Configuration.IPType) == godns.IPV4 {
		recordType = "A"
	} else if strings.ToUpper(handler.Configuration.IPType) == godns.IPV6 {
		recordType = "AAAA"
	}

	log.Println("Querying records with type:", recordType)
	req, client := handler.newRequest("GET", fmt.Sprintf("/zones/"+zoneID+"/dns_records?type=%s&page=1&per_page=500", recordType), nil)
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Request error:", err.Error())
		return empty
	}

	body, _ := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &r)
	if err != nil {
		log.Printf("Decoder error: %+v\n", err)
		log.Printf("Response body: %+v\n", string(body))
		return empty
	}
	if r.Success != true {
		body, _ := ioutil.ReadAll(resp.Body)
		log.Printf("Response failed: %+v\n", string(body))
		return empty

	}
	return r.Records
}

// Update DNS A Record with new IP
func (handler *Handler) updateRecord(record DNSRecord, newIP string)  string {

	var r DNSRecordUpdateResponse
	record.SetIP(newIP)
	var lastIP string

	j, _ := json.Marshal(record)
	req, client := handler.newRequest("PUT",
		"/zones/"+record.ZoneID+"/dns_records/"+record.ID,
		bytes.NewBuffer(j),
	)
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Request error:", err.Error())
		return ""
	}

	body, _ := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &r)
	if err != nil {
		log.Printf("Decoder error: %+v\n", err)
		log.Printf("Response body: %+v\n", string(body))
		return ""
	}
	if r.Success != true {
		body, _ := ioutil.ReadAll(resp.Body)
		log.Printf("Response failed: %+v\n", string(body))
	} else {
		log.Printf("Record updated: %+v - %+v", record.Name, record.IP)
		lastIP = record.IP
	}
	return lastIP
}
