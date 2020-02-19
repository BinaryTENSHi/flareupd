package main

import (
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cloudflare/cloudflare-go"
)

const (
	EnvVarIp4InfoUrl = "IP4_INFO_URL"
	Ip4InfoUrl       = "https://v4.ident.me/"
	EnvVarIp6InfoUrl = "IP6_INFO_URL"
	Ip6InfoUrl       = "https://v6.ident.me/"

	EnvVarCfApiKey   = "CF_API_KEY"
	EnvVarCfEmail    = "CF_API_EMAIL"
	EnvVarCfZoneName = "CF_ZONE_NAME"

	EnvVarRefresh = "REFRESH"
	EnvVarEntry   = "ENTRY"
)

func requiredEnvVariable(variable string) string {
	if val, ok := os.LookupEnv(variable); ok {
		return val
	}

	log.Fatalf("Required environment variable '%s' is not set!", variable)
	return ""
}

func requiredEnvVariableTime(variable string) time.Duration {
	value := requiredEnvVariable(variable)
	t, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("Environment variable '%s' is not a number", variable)
	}
	return time.Duration(t) * time.Second
}

func optionalEnvVariable(variable string, def string) string {
	if val, ok := os.LookupEnv(variable); ok {
		return val
	}

	return def
}

func main() {
	apiKey := requiredEnvVariable(EnvVarCfApiKey)
	email := requiredEnvVariable(EnvVarCfEmail)
	zoneName := requiredEnvVariable(EnvVarCfZoneName)
	refresh := requiredEnvVariableTime(EnvVarRefresh)
	entry := requiredEnvVariable(EnvVarEntry)

	ip4Url := optionalEnvVariable(EnvVarIp4InfoUrl, Ip4InfoUrl)
	ip6Url := optionalEnvVariable(EnvVarIp6InfoUrl, Ip6InfoUrl)

	api, err := cloudflare.New(apiKey, email)
	if err != nil {
		log.Fatalf("Failed to create cloudflare client: %v", err)
	}

	zoneId, err := api.ZoneIDByName(zoneName)
	if err != nil {
		log.Fatalf("Failed to find zone '%s': %v", zoneName, err)
	}

	log.Println("Starting flareupd...")

	ticker := time.Tick(refresh)
	cancel := make(chan os.Signal, 1)
	signal.Notify(cancel, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	ip4Updater := &FlareUpdater{
		Api:     api,
		Fetcher: &IpFetcher{Url: ip4Url},
		Type:    "A",
		ZoneId:  zoneId,
		Name:    entry + "." + zoneName,
	}

	ip6Updater := &FlareUpdater{
		Api:     api,
		Fetcher: &IpFetcher{Url: ip6Url},
		Type:    "AAAA",
		ZoneId:  zoneId,
		Name:    entry + "." + zoneName,
	}

	updaters := make([]*FlareUpdater, 0, 2)

	if ip4Updater.Valid() {
		updaters = append(updaters, ip4Updater)
	} else {
		log.Println("IPv4 updater is not valid: disabled")
	}

	if ip6Updater.Valid() {
		updaters = append(updaters, ip6Updater)
	} else {
		log.Println("IPv6 updater is not valid: disabled")
	}

	for {
		for _, u := range updaters {
			if err := u.UpdateContent(); err != nil {
				log.Printf("Failed to update record: %v", err)
			}
		}

		select {
		case <-ticker:
			continue

		case <-cancel:
			log.Println("Stopping flareupd...")
			os.Exit(0)
		}
	}
}

type FlareUpdater struct {
	Api     *cloudflare.API
	Fetcher *IpFetcher
	Type    string
	ZoneId  string
	Name    string
}

func (f *FlareUpdater) Valid() bool {
	ip, err := f.Fetcher.FetchIp()
	if err != nil {
		log.Printf("Failed to fetch an IP from '%s'", f.Fetcher.Url)
		return false
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		log.Printf("Failed to parse response from '%s' to an IP", f.Fetcher.Url)
		log.Printf(" -- response: %s", ip)
		return false
	}

	return true
}

func (f *FlareUpdater) UpdateContent() error {
	ip, err := f.Fetcher.FetchIp()
	if err != nil {
		return err
	}

	existing, err := f.Api.DNSRecords(f.ZoneId, cloudflare.DNSRecord{Type: f.Type})
	if err != nil {
		return err
	}

	for _, e := range existing {
		if e.Name == f.Name {
			if e.Content == ip {
				return nil
			}

			return f.update(e.ID, ip)
		}
	}

	return f.create(ip)
}

func (f *FlareUpdater) update(id string, ip string) error {
	record := cloudflare.DNSRecord{
		ID:      id,
		Type:    f.Type,
		Name:    f.Name,
		Content: ip,
	}

	log.Printf("Updating record (%s) '%s' to IP '%s'", f.Type, f.Name, ip)
	return f.Api.UpdateDNSRecord(f.ZoneId, id, record)
}

func (f *FlareUpdater) create(ip string) error {
	record := cloudflare.DNSRecord{
		Type:    f.Type,
		Name:    f.Name,
		Content: ip,
	}

	log.Printf("Updating record (%s) '%s' to IP '%s'", f.Type, f.Name, ip)
	_, err := f.Api.CreateDNSRecord(f.ZoneId, record)
	return err
}

type IpFetcher struct {
	Url string
}

func (i *IpFetcher) FetchIp() (string, error) {
	res, err := http.DefaultClient.Get(i.Url)
	if err != nil {
		return "", err
	}

	bs, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(bs)), nil
}
