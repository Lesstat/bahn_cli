package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"strconv"

	resty "gopkg.in/resty.v1"
)

const baseURL = "https://api.deutschebahn.com/timetables/v1"

type stations struct {
	XMLName xml.Name  `xml:"stations"`
	Station []station `xml:"station"`
}
type station struct {
	Name string `xml:"name,attr"`
	ID   int    `xml:"eva,attr"`
}
type timetable struct {
	XMLName xml.Name `xml:"timetable"`
	Trips   []trip   `xml:"s"`
}
type trip struct {
	ID        string `xml:"id,attr"`
	TL        tl     `xml:"tl"`
	Departure dep    `xml:"dp"`
}
type tl struct {
	F string `xml:"f,attr"`
	T string `xml:"t,attr"`
	O string `xml:"o,attr"`
	C string `xml:"c,attr"`
	N string `xml:"n,attr"`
}
type dep struct {
	Time string `xml:"pt,attr"`
	Path string `xml:"ppth,attr"`
}

func main() {

	tokenBytes, err := ioutil.ReadFile("auth_token")
	if err != nil {
		fmt.Printf("could not read token file\n")
		fmt.Printf("error: %s\n", err)
		return
	}
	resty.SetAuthToken(string(tokenBytes))
	station, err := getStation("Heilbronn Hauptbahnhof")
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}
	fmt.Printf("Eva Number: %d\n", station.ID)

	ttable, err := getTimetable(station, "171015", 11)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}
	for _, trip := range ttable.Trips {
		fmt.Printf("%s\n", trip.Departure.Time)
	}

}

func getStation(stationName string) (station, error) {
	resp, err := resty.R().Get(baseURL + "/station/" + stationName)
	var stat stations

	err = xml.Unmarshal(resp.Body(), &stat)
	if err != nil {
		var station station
		return station, err
	}
	return stat.Station[0], nil
}

func getTimetable(station station, date string, hour int) (timetable, error) {
	var ttable timetable
	resp, err := resty.R().Get(baseURL + "/plan/" + strconv.Itoa(station.ID) + "/" + date + "/" + strconv.Itoa(hour))
	if err != nil {
		return ttable, err
	}
	err = xml.Unmarshal(resp.Body(), &ttable)
	if err != nil {
		return ttable, err
	}
	return ttable, nil
}
