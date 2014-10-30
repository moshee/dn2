package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type GeoIPRecord struct {
	Request             string  `json:"geoplugin_request"`
	Status              int     `json:"geoplugin_status"`
	Credit              string  `json:"geoplugin_credit"`
	City                string  `json:"geoplugin_city"`
	Region              string  `json:"geoplugin_region"`
	AreaCode            string  `json:"geoplugin_areaCode"`
	DmaCode             string  `json:"geoplugin_dmaCode"`
	CountryCode         string  `json:"geoplugin_countryCode"`
	CountryName         string  `json:"geoplugin_countryName"`
	ContinentCode       string  `json:"geoplugin_continentCode"`
	Latitude            string  `json:"geoplugin_latitude"`
	Longitude           string  `json:"geoplugin_longitude"`
	RegionCode          string  `json:"geoplugin_regionCode"`
	RegionName          string  `json:"geoplugin_regionName"`
	CurrencyCode        string  `json:"geoplugin_currencyCode"`
	CurrencySymbol      string  `json:"geoplugin_currencySymbol"`
	CurrencySymbol_UTF8 string  `json:"geoplugin_currencySymbol_UTF8"`
	CurrencyConverter   float64 `json:"geoplugin_currencyConverter"`
}

const geopluginURL = "http://geoplugin.net/json.gp?ip="

func geoip(ip string) (*GeoIPRecord, error) {
	resp, err := http.Get(geopluginURL + strings.Split(ip, ":")[0])
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rec := new(GeoIPRecord)
	if err = json.NewDecoder(resp.Body).Decode(rec); err != nil {
		return nil, err
	}
	return rec, nil
}

func geocontinent(ip string) (string, error) {
	rec, err := geoip(ip)
	if err != nil {
		return "", err
	}
	if rec.Status > 299 || rec.Status < 200 {
		log.Println(rec.Status)
		return "", fmt.Errorf("geoplugin response code is %d", rec.Status)
	}
	return rec.ContinentCode, nil
}
