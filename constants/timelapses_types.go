package constants

import (
	"fmt"
	"time"
)

type TimelapseType struct {
	Name               string
	Directory          string
	SubDirectoryNaming func(t time.Time) string
}

var Day = TimelapseType{"DAY", "days_of_year", func(t time.Time) string {
	return t.Format("02-01-2006")
}}
var Week = TimelapseType{"WEEK", "weeks_of_year", func(t time.Time) string {
	year, week := t.ISOWeek()
	return fmt.Sprintf("%d-W%d", year, week)
}}
var Month = TimelapseType{"MONTH", "months_of_year", func(t time.Time) string {
	return t.Format("2006-01")
}}
var Quarter = TimelapseType{"QUARTER", "quarters_of_year", func(t time.Time) string {
	return fmt.Sprintf("%d-Q%d", t.Year(), (int(t.Month())+2)/3)
}}
