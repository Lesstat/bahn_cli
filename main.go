package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	resty "gopkg.in/resty.v1"
)

const baseURL = "https://api.deutschebahn.com/timetables/v1"
const dateFormat = "060102"
const hourFormat = "15"
const depFormat = "0601021504"
const outpFormat = "15:04"

var idReg = regexp.MustCompile(`-?\d+-\d`)

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
	ID        string   `xml:"id,attr"`
	TL        tl       `xml:"tl"`
	Departure halfTrip `xml:"dp"`
	Arrival   halfTrip `xml:"ar"`
}
type tl struct {
	F string `xml:"f,attr"`
	T string `xml:"t,attr"`
	O string `xml:"o,attr"`
	C string `xml:"c,attr"`
	N string `xml:"n,attr"`
}
type halfTrip struct {
	Time string `xml:"pt,attr"`
	Path string `xml:"ppth,attr"`
}

type stop struct {
	station       station
	arrivalTime   time.Time
	departureTime time.Time
}

func main() {
	setUpAuthToken()

	from, _ := getStation("Heilbronn Hauptbahnhof")
	to, _ := getStation("Weinsberg")

	stops, err := fromTo(from, to, time.Now())
	if err != nil {
		return
	}
	fmt.Printf("#, Station, arrival, departure\n")
	for index, stop := range stops {
		fmt.Printf("%d, %s, %s, %s\n", index, stop.station.Name, stop.arrivalTime.Format(outpFormat), stop.departureTime.Format(outpFormat))
	}
}

func getStation(stationName string) (station station, err error) {
	var stat stations
	resp, err := resty.R().Get(baseURL + "/station/" + stationName)
	if err != nil {
		return station, err
	}
	err = xml.Unmarshal(resp.Body(), &stat)
	if err != nil {
		return station, err
	}
	return stat.Station[0], nil
}

func getTimetable(station station, date time.Time) (ttable timetable, err error) {
	callURL := strconv.Itoa(station.ID) + "/" + date.Format(dateFormat) + "/" + date.Format(hourFormat)
	resp, err := resty.R().Get(baseURL + "/plan/" + callURL)
	if err != nil {
		return ttable, err
	}
	err = xml.Unmarshal(resp.Body(), &ttable)
	if err != nil {
		return ttable, err
	}
	return ttable, nil
}

func fromTo(from station, to station, date time.Time) ([]stop, error) {
	var filteredFromTrips []trip
	var filteredToTrips []trip

	fromTrips, err := getTimetable(from, date)
	if err != nil {
		fmt.Printf("Could not get timetable for %s \n", from.Name)
		fmt.Printf("%s\n", err)
		return nil, err
	}
	for _, trip := range fromTrips.Trips {
		if strings.Contains(trip.Departure.Path, to.Name) {
			filteredFromTrips = append(filteredFromTrips, trip)
		}
	}
	sort.Sort(ByDepTime(filteredFromTrips))

	fromStop := stop{station: from, arrivalTime: date}
	var id string
	for _, trip := range filteredFromTrips {
		depTime, err := time.Parse(depFormat, trip.Departure.Time)
		if err != nil {
			return nil, err
		}
		if depTime.Before(date) {
			continue
		} else {
			id = idReg.FindAllString(trip.ID, 1)[0]
			fromStop.departureTime = depTime
		}
	}
	toStop := stop{station: to}
	curDate := date
	counter := 0
	for toStop.arrivalTime.IsZero() {
		if counter > 3 {
			break
		}
		filteredToTrips, err = getAndFilterToTrips(to, from, curDate)
		if err != nil {
			return nil, err
		}
		curDate = curDate.Add(1 * time.Hour)
		counter += 1
		for _, trip := range filteredToTrips {
			if idReg.FindAllString(trip.ID, 1)[0] == id {
				arrTime, err := time.Parse(depFormat, trip.Arrival.Time)
				if err != nil {
					return nil, err
				}
				toStop.arrivalTime = arrTime
			}
		}
	}
	return []stop{fromStop, toStop}, nil
}

type ByArrTime []trip

func (a ByArrTime) Len() int           { return len(a) }
func (a ByArrTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByArrTime) Less(i, j int) bool { return a[i].Arrival.Time < a[j].Arrival.Time }

type ByDepTime []trip

func (a ByDepTime) Len() int           { return len(a) }
func (a ByDepTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByDepTime) Less(i, j int) bool { return a[i].Departure.Time < a[j].Departure.Time }

func getAndFilterToTrips(to station, from station, date time.Time) ([]trip, error) {
	var filteredToTrips []trip
	toTrips, err := getTimetable(to, date)
	if err != nil {
		fmt.Printf("Could not get timetable for %s \n", to.Name)
		fmt.Printf("%s\n", err)
		return nil, err
	}
	for _, trip := range toTrips.Trips {
		if strings.Contains(trip.Arrival.Path, from.Name) {
			filteredToTrips = append(filteredToTrips, trip)
		}
	}
	sort.Sort(ByArrTime(filteredToTrips))

	return filteredToTrips, nil
}

func setUpAuthToken() {
	tokenBytes, err := ioutil.ReadFile("auth_token")
	if err != nil {
		fmt.Printf("could not read token file\n")
		fmt.Printf("error: %s\n", err)
		return
	}
	resty.SetAuthToken(string(tokenBytes))
}
