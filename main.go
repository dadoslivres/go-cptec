package main

import (
	"bytes"
	"database/sql"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"unicode"

	"github.com/k0kubun/pp"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	_ "github.com/mattn/go-sqlite3"
)

func normalizeName(s string) string {
	t := transform.Chain(
		norm.NFD,
		transform.RemoveFunc(func(r rune) bool {
			return unicode.Is(unicode.Mn, r)
		}),
		norm.NFC,
	)
	normalizedName, _, _ := transform.String(t, s)
	return strings.ToLower(normalizedName)
}

type City struct {
	ID    int    `xml:"id"`
	Name  string `xml:"nome"`
	State string `xml:"uf"`
}

type Cities struct {
	City []City `xml:"cidade"`
}

func newUTF8Decoder(body []byte) *xml.Decoder {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.CharsetReader = charset.NewReaderLabel

	return decoder
}

type Forecast struct {
	Day         string `xml:"dia"`
	Climate     string `xml:"tempo"`
	Description string
	Max         string `xml:"maxima"`
	Min         string `xml:"minima"`
	UV          string `xml:"iuv"`
}

type ForecastResult struct {
	Name      string      `xml:"nome"`
	State     string      `xml:"uf"`
	Forecasts []*Forecast `xml:"previsao"`
}

func getCPTECCities(s string) (cities []*City, err error) {
	requestURL := "http://servicos.cptec.inpe.br/XML/listaCidades?city=" + url.QueryEscape(s)
	resp, err := http.Get(requestURL)
	if err != nil {
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	decoder := newUTF8Decoder(body)
	err = decoder.Decode(&result)

	if err != nil {
		return
	}
	cities = result.Cities
	return
}

func getForecast(city *City) (result *ForecastResult, err error) {
	requestURL := fmt.Sprintf("http://servicos.cptec.inpe.br/XML/cidade/%v/previsao.xml", city.ID)
	fmt.Println("GET %s", requestURL)
	resp, err := http.Get(requestURL)
	if err != nil {
		return
	}

	defer resp.Body.Close()

	decoder := newUTF8Decoder(body)
	err = decoder.Decode(&result)
	if err != nil {
		return
	}
	return
}

func getListOfCities(db *sql.DB) (cities []string, err error) {
	rows, err := db.Query("select id, name from ibge")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name string
		err = rows.Scan(&id, &name)
		if err != nil {
			log.Fatal(err)
		}
		cities = append(cities, name)
	}
	err = rows.Err()
	return
}

func main() {
	isBuild := len(os.Args) > 1 && os.Args[1:][0] == "build"

	db, err := sql.Open("sqlite3", "./cities.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if isBuild {
		cities, err := getListOfCities()
		if err != nil {
			log.Panic(err)
		}
		fmt.Println(len(cities))

		for _, city := range cities {
			pp.Println(city)
			CPTECCities, err := getCPTECCities(normalizeName(city))
			if err != nil {
				log.Panic(err)
			}
			for _, city := range cities {
				_, err := db.Exec("insert into cptec(id, name, state) values(?,?,?)", city.ID, city.Name, city.State)
				if err != nil {
					log.Panic(err)
				}
			}
		}
	}
}

func forecastString(f *ForecastResult) (s string) {
	s = fmt.Sprintf("%s, %s\n", f.Name, f.State)
	s += singleForecastString(f.Forecasts[0], "Hoje")
	s += singleForecastString(f.Forecasts[1], "Amanhã")
	s += singleForecastString(f.Forecasts[2], "Depois de amanhã")
	return
}

func singleForecastString(f *Forecast, day string) string {
	return fmt.Sprintf(
		"*%s*: %s\nMín. %sºC, Máx. %sºC, UV %s\n",
		day,
		friendlyClimate(f),
		f.Min,
		f.Max,
		f.UV,
	)
}

func friendlyClimate(f *Forecast) string {
	climateMap := map[string]string{
		"ec":  "Encoberto com Chuvas Isoladas",
		"ci":  "Chuvas Isoladas",
		"c":   "Chuva",
		"in":  "Instável",
		"pp":  "Poss. de Pancadas de Chuva",
		"cm":  "Chuva pela Manhã",
		"cn":  "Chuva a Noite",
		"pt":  "Pancadas de Chuva a Tarde",
		"pm":  "Pancadas de Chuva pela Manhã",
		"np":  "Nublado e Pancadas de Chuva",
		"pc":  "Pancadas de Chuva",
		"pn":  "Parcialmente Nublado",
		"cv":  "Chuvisco",
		"ch":  "Chuvoso",
		"t":   "Tempestade",
		"ps":  "Predomínio de Sol",
		"e":   "Encoberto",
		"n":   "Nublado",
		"cl":  "Céu Claro",
		"nv":  "Nevoeiro",
		"g":   "Geada",
		"ne":  "Neve",
		"nd":  "Não Definido",
		"pnt": "Pancadas de Chuva a Noite",
		"psc": "Possibilidade de Chuva",
		"pcm": "Possibilidade de Chuva pela Manhã",
		"pct": "Possibilidade de Chuva a Tarde",
		"pcn": "Possibilidade de Chuva a Noite",
		"npt": "Nublado com Pancadas a Tarde",
		"npn": "Nublado com Pancadas a Noite",
		"ncn": "Nublado com Poss. de Chuva a Noite",
		"nct": "Nublado com Poss. de Chuva a Tarde",
		"ncm": "Nubl. c/ Poss. de Chuva pela Manhã",
		"npm": "Nublado com Pancadas pela Manhã",
		"npp": "Nublado com Possibilidade de Chuva",
		"vn":  "Variação de Nebulosidade",
		"ct":  "Chuva a Tarde",
		"ppn": "Poss. de Panc. de Chuva a Noite",
		"ppt": "Poss. de Panc. de Chuva a Tarde",
		"ppm": "Poss. de Panc. de Chuva pela Manhã",
	}

	emojiMap := map[string]string{
		"ec":  "🌦",
		"ci":  "🌦",
		"c":   "🌧",
		"in":  "🌦",
		"pp":  "🌦",
		"cm":  "🌧",
		"cn":  "🌧",
		"pt":  "🌦",
		"pm":  "🌦",
		"np":  "🌦",
		"pc":  "🌦",
		"pn":  "🌤",
		"cv":  "🌧",
		"ch":  "🌧",
		"t":   "⛈",
		"ps":  "☀",
		"e":   "⛅",
		"n":   "🌥",
		"cl":  "☀",
		"nv":  "🌫",
		"g":   "❄",
		"ne":  "☃",
		"nd":  "",
		"pnt": "🌧",
		"psc": "🌧",
		"pcm": "🌧",
		"pct": "🌧",
		"pcn": "🌧",
		"npt": "🌧",
		"npn": "🌧",
		"ncn": "🌧",
		"nct": "🌧",
		"ncm": "🌧",
		"npm": "🌧",
		"npp": "🌧",
		"vn":  "🌥",
		"ct":  "🌧",
		"ppn": "🌧",
		"ppt": "🌧",
		"ppm": "🌧",
	}
	if f.Description != "" {
		return emojiMap[f.Climate] + " " + f.Description
	}
	return emojiMap[f.Climate] + " " + climateMap[f.Climate]
}

func getCity(db *sql.DB, str string) (city *City, err error) {
	city = &City{}
	stmt, err := db.Prepare("select ID, Name, State from cptec where cptec = ?")
	if err != nil {
		return
	}
	defer stmt.Close()
	err = stmt.QueryRow(str).Scan(&city.ID, &city.Name, &city.State)
	if err != nil {
		return
	}
	return
}
