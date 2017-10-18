package main

import (
	"encoding/xml"
	"time"
)

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
