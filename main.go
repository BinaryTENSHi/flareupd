package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	CloudflareUrl = "https://api.cloudflare.com/client/v4"
	IpInfoUrl     = "https://ipinfo.io/ip"

	EnvVarCfApiKey = "CF_API_KEY"
	EnvVarCfEmail  = "CF_API_EMAIL"
	EnvVarCfZoneId = "CF_ZONE_ID"

	EnvVarRefresh = "REFRESH"
	EnvVarRecord  = "RECORD"
)

type vars struct {
	ApiKey  string
	Email   string
	ZoneId  string
	Refresh time.Duration
	Record  string
}

type record struct {
	Id      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

type records struct {
	Success bool     `json:"success"`
	Result  []record `json:"result"`
}

func ensureEnvVariable(variable string) string {
	value := strings.TrimSpace(os.Getenv(variable))
	if len(value) == 0 {
		log.Fatalf("Required environment variable '%s' is not set!", variable)
	}

	return value
}

func ensureEnvVariableTime(variable string) time.Duration {
	value := ensureEnvVariable(variable)
	t, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("Environment variable '%s' is not a number", variable)
	}
	return time.Duration(t) * time.Second
}

func externalIp() (string, error) {
	resp, err := http.DefaultClient.Get(IpInfoUrl)
	if err != nil {
		return "", err
	}

	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(bs)), nil
}

func updateDomain(vars vars) error {
	external, err := externalIp()
	if err != nil {
		return err
	}

	log.Printf("External IP is '%s'. Updating record...", external)

	return updateRecord(vars, external)
}

func getRecords(vars vars) ([]record, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records", CloudflareUrl, vars.ZoneId)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("X-Auth-Email", vars.Email)
	req.Header.Add("X-Auth-Key", vars.ApiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	records := &records{}
	if err := json.NewDecoder(resp.Body).Decode(records); err != nil {
		return nil, err
	}

	return records.Result, nil
}

func updateRecord(vars vars, external string) error {
	records, err := getRecords(vars)
	if err != nil {
		return err
	}

	method := http.MethodPost
	url := fmt.Sprintf("%s/zones/%s/dns_records", CloudflareUrl, vars.ZoneId)
	for _, r := range records {
		if r.Name == vars.Record {
			log.Printf("Record exists '%s'", r.Id)
			url = fmt.Sprintf("%s/%s", url, r.Id)
			method = http.MethodPut
		}
	}

	record := record{
		Type:    "A",
		Name:    vars.Record,
		Content: external,
	}

	bs, err := json.Marshal(record)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(bs))
	if err != nil {
		return err
	}

	req.Header.Add("X-Auth-Email", vars.Email)
	req.Header.Add("X-Auth-Key", vars.ApiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	log.Println(resp.Status)
	return nil
}

func main() {
	vars := vars{}
	vars.ApiKey = ensureEnvVariable(EnvVarCfApiKey)
	vars.Email = ensureEnvVariable(EnvVarCfEmail)
	vars.ZoneId = ensureEnvVariable(EnvVarCfZoneId)
	vars.Refresh = ensureEnvVariableTime(EnvVarRefresh)
	vars.Record = ensureEnvVariable(EnvVarRecord)

	log.Println("Starting flareupd...")

	ticker := time.Tick(vars.Refresh)
	cancel := make(chan os.Signal)
	signal.Notify(cancel, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Immediately update the record
	if err := updateDomain(vars); err != nil {
		log.Printf("Failed to update domain: %v", err)
	}

	for {
		select {
		case <-ticker:
			log.Println("Updating...")
			if err := updateDomain(vars); err != nil {
				log.Printf("Failed to update domain: %v", err)
			}

		case <-cancel:
			log.Println("Shutting down")
			os.Exit(0)
		}
	}
}
