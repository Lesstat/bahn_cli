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

	"github.com/lesstat/bahn_cli/cache"

	resty "gopkg.in/resty.v1"
)

const baseURL = "https://api.deutschebahn.com/timetables/v1"
const dateFormat = "060102"
const hourFormat = "15"
const depFormat = "0601021504"
const outpFormat = "15:04"
const timeFormat = "1504"

var idReg = regexp.MustCompile(`-?\d+-\d`)

type bahnAPI struct {
	err error
	c   cache.Cache
}

func main() {
	flag.Parse()
	args := flag.Args()

	var curTime time.Time
	var err error
	var b bahnAPI
	go b.c.ClearCache()
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
	if len(args) > 1 {
		timeArg := args[1]
		curTime, err = buildTime(timeArg)
		if err != nil {
			fmt.Printf("could not parse %s as time\n", curTime)
		}

	} else {
		curTime = time.Now()
	}

	me, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	stops := b.searchRoute(path.Join(me.HomeDir, ".config/bahn/routes", route), curTime)

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
	var stat stations
	var content []byte
	var err error

	if b.err != nil {
		return station
	}
	url := "/station/" + stationName
	if content, err = b.c.ReadCache(url); err != nil {
		resp, err := resty.R().Get(baseURL + url)
		if err != nil {
			b.err = err
			return station
		}
		content = resp.Body()
		b.c.WriteCache(url, content)
	}
	err = xml.Unmarshal(content, &stat)
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
	var content []byte
	var err error

	callURL := "/plan/" + strconv.Itoa(station.ID) + "/" + date.Format(dateFormat) + "/" + date.Format(hourFormat)

	if content, err = b.c.ReadCache(callURL); err != nil {
		resp, err := resty.R().Get(baseURL + callURL)
		if err != nil {
			b.err = err
			return ttable
		}
		content = resp.Body()
		b.c.WriteCache(callURL, content)
	}
	err = xml.Unmarshal(content, &ttable)
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

	me, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	config := path.Join(me.HomeDir, ".config/bahn/config")

	tokenBytes, err := ioutil.ReadFile(config)
	if err != nil {
		b.err = err
		fmt.Printf("could not read token file\n")
		fmt.Printf("error: %s\n", err)
		return
	}
	resty.SetAuthToken(string(tokenBytes))
}

func (b *bahnAPI) searchRoute(path string, t time.Time) []stop {
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
	curDate := t

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

func buildTime(tString string) (time.Time, error) {
	now := time.Now()
	tTime, err := time.ParseInLocation(timeFormat, tString, time.Local)
	if err != nil {
		return now, err
	}
	return time.Date(now.Year(), now.Month(), now.Day(), tTime.Hour(), tTime.Minute(), 0, 0, time.Local), nil
}
