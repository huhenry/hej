package filter

import (
	"strconv"
	"strings"

	"github.com/huhenry/hej/pkg/common"
	v1beta1 "github.com/huhenry/hej/pkg/microapp/v1beta1"
)

type Filter interface {
	Filte(microApp *v1beta1.MicroApp) *v1beta1.MicroApp
}
type CreatorFilter struct {
	Creator string
}

func (filter CreatorFilter) Filte(microApp *v1beta1.MicroApp) *v1beta1.MicroApp {

	if microApp == nil {
		return nil
	}

	if common.ParamIsFit(filter.Creator, microApp.Creator) {
		return microApp
	}

	return nil
}

type NameFilter struct {
	Name string
}

func (filter NameFilter) Filte(microApp *v1beta1.MicroApp) *v1beta1.MicroApp {
	if microApp == nil {
		return nil
	}

	if strings.Contains(microApp.Name, filter.Name) {
		return microApp
	}

	return nil
}

type TimeFilter struct {
	StartTime string
	EndTime   string
}

func (filter TimeFilter) Filte(microApp *v1beta1.MicroApp) *v1beta1.MicroApp {

	if microApp == nil {
		return nil
	}
	start_time := filter.StartTime
	end_time := filter.EndTime

	startTime, err := strconv.ParseInt(start_time, 10, 64)
	if err != nil {
		logger.Errorf("filter starttime convrt err %s", err)

	}
	endTime, err := strconv.ParseInt(end_time, 10, 64)
	if err != nil {
		logger.Errorf("filter endtime convert err %s", err)

	}

	if microApp.CreateTimeSec >= startTime && microApp.CreateTimeSec <= endTime {
		return microApp
	}

	return nil

}
