package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
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
	Line string `xml:"l,attr"`
}

type stop struct {
	station       station
	arrivalTime   time.Time
	departureTime time.Time
	line          string
}

func main() {
	setUpAuthToken()
	w := tabwriter.NewWriter(os.Stdout, 5, 3, 3, ' ', 0)
	// ellhofen, _ :=  getStation("TELI")
	uni, _ := getStation("Stuttgart Universi")
	stut_tief, _ := getStation("TS  T")
	stut_hbf, _ := getStation("Stuttgart Hbf")
	hn_hbf, _ := getStation("Heilbronn Hbf")

	stops, err := fromTo(uni, stut_tief, time.Now())
	if err != nil {
		return
	}
	new_arr := stops[1].arrivalTime.Add(7 * time.Minute)

	stops2, err := fromTo(stut_hbf, hn_hbf, new_arr)
	if err != nil {
		return
	}
	stops = append(stops, stops2...)

	fmt.Fprintln(w, "# Station \t arrival \t departure \t line")
	for index, stop := range stops {
		if stop.departureTime.IsZero() {
			stop.departureTime = stop.arrivalTime
		}
		fmt.Fprintf(w, "%d %s \t %s \t %s \t %s\n", index, stop.station.Name, stop.arrivalTime.Format(outpFormat), stop.departureTime.Format(outpFormat), stop.line)
	}
	w.Flush()
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
	if stat.Station[0].ID == 0 {
		return station, errors.New("Did not find station for " + stationName)
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
		fmt.Printf("URL %s\n", baseURL+"/plan/"+callURL)
		fmt.Printf("%s\n", resp)
		return ttable, err
	}
	return ttable, nil
}

func fromTo(from station, to station, date time.Time) ([]stop, error) {
	curDate := date.Add(-1 * time.Hour)
	fromStop := stop{station: from, arrivalTime: date}
	var id string

	for fromStop.departureTime.IsZero() {
		curDate = curDate.Add(1 * time.Hour)
		filteredFromTrips, err := getAndFilterTrips(from, to.Name, true, curDate)
		if err != nil {
			return nil, err
		}
		for _, trip := range filteredFromTrips {
			depTime, err := time.ParseInLocation(depFormat, trip.Departure.Time, time.Local)
			if err != nil {
				return nil, err
			}
			if depTime.Before(date) {
				continue
			} else {
				id = idReg.FindAllString(trip.ID, 1)[0]
				fromStop.departureTime = depTime
				fromStop.line = trip.TL.C + trip.Departure.Line
				break
			}
		}
	}
	toStop := stop{station: to}
	counter := 0
	for toStop.arrivalTime.IsZero() {
		if counter > 3 {
			break
		}
		filteredToTrips, err := getAndFilterTrips(to, from.Name, false, curDate)
		if err != nil {
			return nil, err
		}
		curDate = curDate.Add(1 * time.Hour)
		counter += 1
		for _, trip := range filteredToTrips {
			if idReg.FindAllString(trip.ID, 1)[0] == id {
				arrTime, err := time.ParseInLocation(depFormat, trip.Arrival.Time, time.Local)
				if err != nil {
					return nil, err
				}
				toStop.arrivalTime = arrTime
				break
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

func getAndFilterTrips(table station, filterBy string, departure bool, date time.Time) ([]trip, error) {
	var filteredTrips []trip
	trips, err := getTimetable(table, date)
	if err != nil {
		fmt.Printf("Could not get timetable for %s \n", table.Name)
		fmt.Printf("%s\n", err)
		return nil, err
	}
	for _, trip := range trips.Trips {
		if departure {
			if strings.Contains(trip.Departure.Path, filterBy) {
				filteredTrips = append(filteredTrips, trip)
			}
		} else {
			if strings.Contains(trip.Arrival.Path, filterBy) {
				filteredTrips = append(filteredTrips, trip)
			}
		}
	}
	sort.Sort(ByArrTime(filteredTrips))

	return filteredTrips, nil
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

func searchRoute(path string) ([]stop, error) {
	var result []stop
	route, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	route_parts := strings.Split(string(route), "\n")
	curDate := time.Now()
	var from station
	var to station
	for i, part := range route_parts {
		if dur, err := time.ParseDuration(part); err != nil {
			if i == 0 {
				to, err = getStation(part)
				if err != nil {
					return nil, err
				}
			} else {
				from = to
				to, err = getStation(part)
				if err != nil {
					return nil, err
				}
				stops, err := fromTo(from, to, curDate)
				if err != nil {
					return nil, err
				}
				curDate = stops[1].arrivalTime
				result = append(result, stops...)
			}
		} else {
			curDate = curDate.Add(dur)
		}

	}

	return result, nil
}
