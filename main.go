package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

var (
	updateFrequency = flag.Duration("f", 5*time.Minute, "Time between updates")
	apiServer       = flag.String("s", "api.dnsimple.com", "DNSimple API endpoint")
	domainToken     = flag.String("t", "", "Value for X-DNSimple-Domain-Token header")
	domainName      = flag.String("d", "", "Domain the entry is for")
	entryName       = flag.String("n", "", "Name of the entry")
	help            = flag.Bool("h", false, "Show this help")
)

//go:generate gen
// +gen slice:"Where"
type Record struct {
	Record struct {
		ID       int    `json:"id,omitempty"`
		Name     string `json:"name"`
		TTL      int    `json:"ttl,omitempty"`
		Created  string `json:"created_at,omitempty"`
		Updated  string `json:"updated_at,omitempty"`
		DomainID int    `json:"domain_id,omitempty"`
		Content  string `json:"content"`
		Type     string `json:"record_type"`
	} `json:"record"`
}

func main() {
	flag.Parse()

	if *help {
		flag.PrintDefaults()
		return
	}

	if *domainToken == "" || *domainName == "" || *entryName == "" {
		log.Fatalf("-t, -d and -n must be set")
	}

	// Don't wait on the very first run
	d := 0 * time.Second
	for {
		time.Sleep(d)
		d = *updateFrequency

		ip, err := externalIP()
		if err != nil {
			log.Printf("Could not obtain external IP: %s", err)
			continue
		}
		log.Printf("External IP: %s", ip)

		recs, err := listRecords()
		if err != nil {
			log.Printf("Could not list records: %s", err)
			continue
		}

		aRecs := recs.Where(func(r Record) bool {
			return r.Record.Name == *entryName
		}).Where(func(r Record) bool {
			return r.Record.Type == "A"
		})

		switch len(aRecs) {
		case 0:
			log.Printf("Creating new A record %s.%s", *entryName, *domainName)
			if err := createRecord(ip); err != nil {
				log.Printf("Could not create record: %s", err)
			}
		case 1:
			log.Printf("Updating existing A record %s.%s", *entryName, *domainName)
			if err := updateRecord(aRecs[0], ip); err != nil {
				log.Printf("Could not update record: %s", err)
			}
		case 2:
			log.Printf("Multiple A records matching. Skipping")
		}
	}
}

func externalIP() (string, error) {
	resp, err := http.Get("http://jsonip.com")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	obj := map[string]interface{}{}
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return "", err
	}
	rawIp, ok := obj["ip"]
	if !ok {
		return "", fmt.Errorf("No IP field in response")
	}
	ip, ok := rawIp.(string)
	if !ok {
		return "", fmt.Errorf("IP has unexpected type")
	}
	return ip, nil
}

func listRecords() (RecordSlice, error) {
	req, _ := http.NewRequest("GET", fmt.Sprintf("https://%s/v1/domains/%s/records", *apiServer, *domainName), nil)
	authenticate(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	recs := RecordSlice{}
	err = json.NewDecoder(resp.Body).Decode(&recs)
	return recs, err
}

func createRecord(ip string) error {
	rec := Record{}
	rec.Record.Name = *entryName
	rec.Record.Type = "A"
	rec.Record.Content = ip
	rec.Record.TTL = 5
	data, _ := json.Marshal(rec)

	req, _ := http.NewRequest("POST", fmt.Sprintf("https://%s/v1/domains/%s/records", *apiServer, *domainName), bytes.NewReader(data))
	authenticate(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 201 {
		return fmt.Errorf("Record creation failed: %s (%d)", resp.Status, resp.StatusCode)
	}
	return nil
}

func updateRecord(rec Record, ip string) error {
	rec.Record.TTL = 5
	rec.Record.Content = ip
	data, _ := json.Marshal(rec)

	req, _ := http.NewRequest("PUT", fmt.Sprintf("https://%s/v1/domains/%s/records/%d", *apiServer, *domainName, rec.Record.ID), bytes.NewReader(data))
	authenticate(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("Record update failed: %s (%d)", resp.Status, resp.StatusCode)
	}
	return nil
}

func authenticate(req *http.Request) {
	req.Header.Add("Accepts", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-DNSimple-Domain-Token", *domainToken)
	req.Close = true
}
