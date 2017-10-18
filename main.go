package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"
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

type bahnAPI struct {
	err error
}

func main() {
	flag.Parse()
	args := flag.Args()

	var b bahnAPI
	b.setUpAuthToken()
	if len(args) == 0 {
		fmt.Printf("No route given\n")
		fmt.Printf("USAGE: bahn [route] [starttime]\n")
		fmt.Printf("Example:\n")
		fmt.Printf("bahn hw 0730\n")
		fmt.Printf("Means look for the next trip of route hw after 07:30\n")
		return
	}
	route := args[0]

	me, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	stops := b.searchRoute(path.Join(me.HomeDir, ".config/bahn/routes", route))

	if b.err != nil {
		log.Fatal(b.err)
	}

	w := tabwriter.NewWriter(os.Stdout, 5, 3, 3, ' ', 0)
	fmt.Fprintln(w, "# Station \t arrival \t departure \t line")
	for index, stop := range stops {
		if stop.departureTime.IsZero() {
			stop.departureTime = stop.arrivalTime
		}
		fmt.Fprintf(w, "%d %s \t %s \t %s \t %s\n", index, stop.station.Name, stop.arrivalTime.Format(outpFormat), stop.departureTime.Format(outpFormat), stop.line)
	}
	w.Flush()
}
func (b *bahnAPI) getStation(stationName string) station {
	var station station
	if b.err != nil {
		return station
	}
	var stat stations
	resp, err := resty.R().Get(baseURL + "/station/" + stationName)
	if err != nil {
		b.err = err
		return station
	}
	err = xml.Unmarshal(resp.Body(), &stat)
	if err != nil {
		b.err = err
		return station
	}
	if stat.Station[0].ID == 0 {
		b.err = errors.New("Did not find station for " + stationName)
		return station
	}
	return stat.Station[0]
}

func (b *bahnAPI) getTimetable(station station, date time.Time) (ttable timetable) {
	if b.err != nil {
		return ttable
	}

	callURL := strconv.Itoa(station.ID) + "/" + date.Format(dateFormat) + "/" + date.Format(hourFormat)
	resp, err := resty.R().Get(baseURL + "/plan/" + callURL)
	if err != nil {
		b.err = err
		return ttable
	}
	err = xml.Unmarshal(resp.Body(), &ttable)
	if err != nil {
		b.err = err
		return ttable
	}
	return ttable
}

func (b *bahnAPI) fromTo(from station, to station, date time.Time) []stop {
	if b.err != nil {
		return nil
	}

	curDate := date.Add(-1 * time.Hour)
	fromStop := stop{station: from, arrivalTime: date}
	var id string

	counter := 0
	for fromStop.departureTime.IsZero() {
		if counter > 3 {
			b.err = errors.New("Could not find route from " + from.Name + " to " + to.Name)
			return nil
		}
		curDate = curDate.Add(1 * time.Hour)
		filteredFromTrips := b.getAndFilterTrips(from, to.Name, true, curDate)
		for _, trip := range filteredFromTrips {
			depTime, err := time.ParseInLocation(depFormat, trip.Departure.Time, time.Local)
			if err != nil {
				b.err = err
				return nil
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
		counter++
	}
	toStop := stop{station: to}
	counter = 0
	for toStop.arrivalTime.IsZero() {
		if counter > 3 {
			break
		}
		filteredToTrips := b.getAndFilterTrips(to, from.Name, false, curDate)
		curDate = curDate.Add(1 * time.Hour)
		counter++
		for _, trip := range filteredToTrips {
			if idReg.FindAllString(trip.ID, 1)[0] == id {
				arrTime, err := time.ParseInLocation(depFormat, trip.Arrival.Time, time.Local)
				if err != nil {
					b.err = err
					return nil
				}
				toStop.arrivalTime = arrTime
				break
			}
		}
	}
	return []stop{fromStop, toStop}
}

type byArrTime []trip

func (a byArrTime) Len() int           { return len(a) }
func (a byArrTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byArrTime) Less(i, j int) bool { return a[i].Arrival.Time < a[j].Arrival.Time }

type byDepTime []trip

func (a byDepTime) Len() int           { return len(a) }
func (a byDepTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byDepTime) Less(i, j int) bool { return a[i].Departure.Time < a[j].Departure.Time }

func (b *bahnAPI) getAndFilterTrips(table station, filterBy string, departure bool, date time.Time) []trip {
	if b.err != nil {
		return nil
	}

	var filteredTrips []trip
	trips := b.getTimetable(table, date)
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
	sort.Sort(byArrTime(filteredTrips))

	return filteredTrips
}

func (b *bahnAPI) setUpAuthToken() {
	if b.err != nil {
		return
	}
	tokenBytes, err := ioutil.ReadFile("auth_token")
	if err != nil {
		b.err = err
		fmt.Printf("could not read token file\n")
		fmt.Printf("error: %s\n", err)
		return
	}
	resty.SetAuthToken(string(tokenBytes))
}

func (b *bahnAPI) searchRoute(path string) []stop {
	if b.err != nil {
		return nil
	}
	var result []stop
	route, err := ioutil.ReadFile(path)
	if err != nil {
		b.err = err
		return nil
	}
	routeParts := strings.Split(string(route), "\n")
	curDate := time.Now()
	var from station
	var to station
	durAdded := false
	for i, part := range routeParts {
		if dur, err := time.ParseDuration(part); err != nil {
			if i == 0 || durAdded {
				to = b.getStation(part)
				durAdded = false
			} else {
				from = to
				to = b.getStation(part)
				stops := b.fromTo(from, to, curDate)
				if b.err != nil {
					return nil
				}
				curDate = stops[1].arrivalTime
				result = append(result, stops...)
			}
		} else {
			curDate = curDate.Add(dur)
			durAdded = true
		}

	}
	return result
}
