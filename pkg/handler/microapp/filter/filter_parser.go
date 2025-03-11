package filter

import (
	"github.com/huhenry/hej/pkg/log"
	"github.com/kataras/iris/v12"
)

var logger = log.RegisterScope("microapp-filter")

type Parser interface {
	Parse(filterlist []Filter) []Filter
}

type NameParser struct {
	NameFilter
}

func (parser NameParser) Parse(filterlist []Filter) []Filter {
	name := parser.Name
	if len(name) > 0 {
		filterlist = append(filterlist, NameFilter{Name: name})
	}
	return filterlist

}

type CreatorParser struct {
	CreatorFilter
}

func (parser CreatorParser) Parse(filterlist []Filter) []Filter {
	creator := parser.Creator
	if len(creator) > 0 {
		filterlist = append(filterlist, CreatorFilter{Creator: creator})
	}
	return filterlist

}

type TimeParser struct {
	TimeFilter
}

func (parser TimeParser) Parse(filterlist []Filter) []Filter {

	startTime := parser.StartTime
	endTime := parser.EndTime
	if len(startTime) > 0 && len(endTime) > 0 {
		filterlist = append(filterlist, TimeFilter{StartTime: startTime, EndTime: endTime})
	}
	return filterlist

}

func ParserParams(ctx iris.Context) []Filter {

	filterlist := make([]Filter, 0)
	name := ctx.URLParam("name")
	creator := ctx.URLParam("creator")
	startTime := ctx.URLParam("start_time")
	endTime := ctx.URLParam("end_time")

	nameParser := NameParser{NameFilter{Name: name}}
	filterlist = nameParser.Parse(filterlist)
	creatorParser := CreatorParser{CreatorFilter{Creator: creator}}
	filterlist = creatorParser.Parse(filterlist)
	timeParser := TimeParser{TimeFilter{StartTime: startTime, EndTime: endTime}}
	filterlist = timeParser.Parse(filterlist)
	return filterlist
}
