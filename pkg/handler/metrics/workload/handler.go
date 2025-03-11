package workload

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"time"

	"github.com/huhenry/hej/pkg/prometheus"

	corev1 "k8s.io/api/core/v1"

	v1 "github.com/huhenry/hej/pkg/backend/v1"

	"github.com/huhenry/hej/pkg/common"

	"github.com/huhenry/hej/pkg/metrics/pro"

	micro "github.com/huhenry/hej/pkg/microapp"

	"github.com/huhenry/hej/pkg/common/concurrent"

	context2 "github.com/kataras/iris/v12/context"

	prom_v1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/huhenry/hej/pkg/metrics"

	customErrors "github.com/huhenry/hej/pkg/errors"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/log"
	"github.com/huhenry/hej/pkg/multiCluster"
	"github.com/kataras/iris/v12"
)

const (
	QueryTimeout    = 60 * time.Second
	DefaultDuration = 3 * time.Minute

	CPUUnitCore         = "Core"
	CPUUnitMillionsCore = "m"
)

var (
	logger = log.RegisterScope("workload-metric")
)

type Params struct {
	StartTs     int64
	EndTs       int64
	DurationSec int64
	StepSec     int64
	Workload    string
	Namespace   string
	Cluster     string
	PodName     string
}

func fetchParams(ctx iris.Context) (*Params, error) {
	params := &Params{}

	endTime, err := ctx.URLParamInt64(metrics.PathParameterEndTime)
	if err != nil {
		if err == context2.ErrNotFound {
			endTime = time.Now().Unix()
		} else {
			handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, "非法的结束时间")
			return params, err
		}
	}
	now := time.Now().Unix()
	if endTime > now {
		endTime = now
	}

	params.EndTs = endTime
	startTime, err := ctx.URLParamInt64(metrics.PathParameterStartTime)
	if err != nil {
		if err == context2.ErrNotFound {
			startTime = endTime - 1*60*60 // 1 hour
			if startTime == 0 {
				startTime = 0
			}
		} else {
			handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, "非法的开始时间")
			return params, err
		}
	}
	params.StartTs = startTime
	if params.StartTs >= params.EndTs {
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, "结束时间必须大于开始时间")
		return params, fmt.Errorf("结束时间必须大于开始时间")
	}

	duration := ctx.URLParam("duration")
	d, err := strconv.ParseInt(duration, 10, 64)
	if err != nil {
		logger.Warnf("parse step err : %v", err)
		d = int64(DefaultDuration.Seconds())
	}
	params.DurationSec = d

	step := ctx.URLParam("step")
	if len(step) > 0 {
		if s, err := strconv.Atoi(step); err == nil {
			params.StepSec = int64(s)
		}
	}
	if params.StepSec <= 0 {
		params.StepSec = pro.AdaptiveStep(params.StartTs, params.EndTs, params.StepSec)
	}

	//params.Workload = ctx.Params().Get("name")
	appCtx := handler.ExtractAppContext(ctx)
	params.Namespace = appCtx.KubeNamespace
	params.Cluster = appCtx.ClusterName

	return params, nil
}

func GetMetrics(mgr multiCluster.Manager, ctx iris.Context) {
	background := context.Background()
	params, err := fetchParams(ctx)
	if err != nil {
		//handler.ResponseErr(ctx, err)
		return
	}
	logger.Debugf("workload params is : %v", params)
	projectId := ctx.Params().Get("app")
	pods, err := micro.Resource().GetDeploymentPods(background, params.Cluster, params.Namespace, ctx.Params().Get("name"))
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}
	appCtx := handler.ExtractAppContext(ctx)
	clusterName := appCtx.ClusterName
	p8sClient, err := prometheus.NewP8sClient(mgr, clusterName)
	if err != nil {
		logger.Errorf("prometheus Newclient err %v", err)
		msg := fmt.Sprintf("prometheus connection failed : %s", err)
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, msg)
		return

	}
	m := make([]*Metric, 0, len(pods))
	runners := make([]concurrent.Runner, 0, len(pods))

	for i := range pods {
		createTime := pods[i].CreationTimestamp.Time.Unix()
		m = append(m, &Metric{
			Name:              pods[i].Name,
			CreationTimestamp: createTime,
			Status:            GetPodStatus(pods[i]),
			CpuUsage: MetricCPUView{
				Samples: make([][]interface{}, 0),
			},
			MemUsage: MetricView{
				Samples: make([][]interface{}, 0),
			},
		})
		runners = append(runners, getPodMetrics(params, p8sClient, pods[i].Name, projectId, m, i))
	}
	err = concurrent.Run(background, QueryTimeout, runners...)

	if err != nil {
		if err == concurrent.ErrTimeout {
			handler.Response(ctx, customErrors.StatusCodeServiceError, "请求数据超时")
			return
		}
		handler.ResponseErr(ctx, err)
		return
	}

	handler.ResponseOk(ctx, m)

}

func getPodMetricByType(m []*Metric, i int, types string, p8sclient *prometheus.Client, startTime, endTime, step int64, podName, kubeNamespace string) concurrent.Runner {
	return func(ctx context.Context) (concurrent.CompleteFunc, error) {
		return func(ctx context.Context) error {
			value := &v1.Data{}
			var err error
			if types == v1.Cpu {
				if value, err = p8sclient.GetPodCpuMetrics(startTime, endTime, step, podName, kubeNamespace); err != nil {
					return err
				}
				logger.Debugf("cpu is %s ", value)
				if !reflect.ValueOf(value.Correct).IsZero() && !reflect.ValueOf(value.Correct.Scale).IsZero() && len(value.Correct.Scale.Result) > 0 {
					m[i].CpuUsage.Samples = value.Correct.Scale.Result[0].Values
					logger.Debugf("cpuUsage is %v", value.Correct.Scale.Result[0].Values)
					m[i].CpuUsage.Current.Unit = value.Correct.Scale.Unit
				}
			}
			if types == v1.Memory {
				if value, err = p8sclient.GetPodMemoryMetrics(startTime, endTime, step, podName, kubeNamespace); err != nil {
					return err
				}
				logger.Debugf("memory is %s ", value)
				if !reflect.ValueOf(value.Correct).IsZero() && !reflect.ValueOf(value.Correct.Scale).IsZero() && len(value.Correct.Scale.Result) > 0 {
					m[i].MemUsage.Samples = value.Correct.Scale.Result[0].Values
					logger.Debugf("memoryUsage is %f", value.Correct.Scale.Result[0].Values)
				}
			}

			return nil
		}, nil
	}
}

func getPodMetrics(params *Params, p8sclient *prometheus.Client, podName, projectId string, m []*Metric, i int) concurrent.Runner {
	return func(ctx context.Context) (concurrent.CompleteFunc, error) {
		nowTime := time.Now().Unix() //当前时间戳
		return func(_ context.Context) error {
			runners := make([]concurrent.Runner, 0, 2)
			background := context.Background()
			// 获取cpu指标列表
			var err error
			runners = append(runners, getPodMetricByType(m, i, v1.Cpu, p8sclient, params.StartTs, params.EndTs, params.StepSec, podName, params.Namespace))

			// 获取内存指标列表
			runners = append(runners, getPodMetricByType(m, i, v1.Memory, p8sclient, params.StartTs, params.EndTs, params.StepSec, podName, params.Namespace))
			//获取当前时间cpu占用
			currentCpu := &v1.Data{}
			if currentCpu, err = p8sclient.GetPodCpuMetrics(nowTime, nowTime, params.StepSec, podName, params.Namespace); err != nil {
				return err
			}
			//获取当前时间内存使用
			currentMemory := &v1.Data{}
			if currentMemory, err = p8sclient.GetPodMemoryMetrics(nowTime, nowTime, params.StepSec, podName, params.Namespace); err != nil {
				return err
			}
			logger.Debugf("podnames is %s ", podName)
			logger.Debugf("currentCpu is %s ", currentCpu)
			if currentCpu != nil && !reflect.ValueOf(currentCpu.Correct).IsZero() && !reflect.ValueOf(currentCpu.Correct.Scale).IsZero() && len(currentCpu.Correct.Scale.Result) > 0 {
				var value float64
				if value, err = strconv.ParseFloat(currentCpu.Correct.Scale.Result[0].Values[0][1].(string), 64); err != nil {
					return err
				}
				m[i].CpuUsage.Current = CPUMeasurement{
					Value: value,
					Unit:  currentCpu.Correct.Scale.Unit,
				}
			}
			logger.Debugf("currentMemory is %s ", currentMemory)
			if currentMemory != nil && !reflect.ValueOf(currentMemory.Correct).IsZero() && !reflect.ValueOf(currentMemory.Correct.Scale).IsZero() && len(currentMemory.Correct.Scale.Result) > 0 {
				var value float64
				if value, err = strconv.ParseFloat(currentMemory.Correct.Scale.Result[0].Values[0][1].(string), 64); err != nil {
					return err
				}
				m[i].MemUsage.Current = value
			}
			err = concurrent.Run(background, QueryTimeout, runners...)

			if err != nil {
				if err == concurrent.ErrTimeout {
					return fmt.Errorf("请求数据超时")
				}
				return err
			}

			return nil
		}, nil
	}
}

func GetPodStatus(pod *corev1.Pod) PodStatus {
	status := PodStatus{
		Phase:      "terminating",
		Conditions: pod.Status.Conditions,
	}
	if pod.DeletionTimestamp.IsZero() {
		status.Phase = string(pod.Status.Phase)
	}

	return status
}

func convertParamToMap(params *Params, resultParams map[string]string) {
	resultParams["start"] = strconv.FormatInt(params.StartTs, 10)
	resultParams["end"] = strconv.FormatInt(params.EndTs, 10)
	resultParams["step"] = strconv.FormatInt(params.StepSec, 10)
	resultParams["k8s_name"] = params.Cluster
	resultParams["kube_namespace"] = params.Namespace
	resultParams["pod_name"] = params.PodName
}

func copyParams(source *Params, podName string) (target *Params) {
	target = &Params{
		StartTs:     source.StartTs,
		EndTs:       source.EndTs,
		DurationSec: source.DurationSec,
		StepSec:     source.StepSec,
		Namespace:   source.Namespace,
		Cluster:     source.Cluster,
		PodName:     podName,
	}

	return
}

type Metric struct {
	Name              string        `json:"name"`
	Status            PodStatus     `json:"status"`
	CpuUsage          MetricCPUView `json:"cpuUsage"`
	MemUsage          MetricView    `json:"memUsage"`
	CreationTimestamp int64         `json:"CreationTimestamp"`
}

type PodStatus struct {
	Phase      string                `json:"phase"`
	Conditions []corev1.PodCondition `json:"conditions,omitempty"`
}

type MetricView struct {
	Current float64         `json:"current"`
	Samples [][]interface{} `json:"samples"`
}
type MetricCPUView struct {
	Current CPUMeasurement  `json:"current"`
	Samples [][]interface{} `json:"samples"`
}

type CPUMeasurement struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

func getRangeCpuUsage(api prom_v1.API, params *Params, metrics []*Metric) concurrent.Runner {
	return func(ctx context.Context) (concurrent.CompleteFunc, error) {
		options := pro.RangeQueryOptions{
			StartTs:     params.StartTs,
			EndTs:       params.EndTs,
			DurationSec: params.DurationSec,
			StepSec:     params.StepSec,
		}

		matrix, err := pro.Deployment().RangePodsCpuUsage(ctx, api, params.Namespace, params.PodName, options)
		if err != nil {
			return nil, err
		}

		return func(_ context.Context) error {
			for name, pairs := range matrix {
				var m *Metric
				for i := range metrics {
					if metrics[i].Name == name {
						m = metrics[i]
						break
					}
				}
				if m == nil {
					continue
				}
				pairs = prometheus.GenTimelines(pairs, options.StartTs, options.EndTs, options.StepSec)
				for _, p := range pairs {
					result := make([]interface{}, 0, 2)
					result = append(result, p.Timestamp.Unix(), roundCpu(float64(p.Value)*1000))
					m.CpuUsage.Samples = append(m.CpuUsage.Samples, result)
				}
				if len(pairs) > 0 {
					m.CpuUsage.Current = roundCpuMeasure(float64(pairs[len(pairs)-1].Value))
				}

			}
			return nil
		}, nil
	}
}

func getRangeMemUsage(api prom_v1.API, params *Params, metrics []*Metric) concurrent.Runner {
	return func(ctx context.Context) (concurrent.CompleteFunc, error) {
		options := pro.RangeQueryOptions{
			StartTs:     params.StartTs,
			EndTs:       params.EndTs,
			DurationSec: params.DurationSec,
			StepSec:     params.StepSec,
		}

		matrix, err := pro.Deployment().RangePodsMemUsage(ctx, api, params.Namespace, params.PodName, options)
		if err != nil {
			return nil, err
		}

		return func(_ context.Context) error {
			for name, pairs := range matrix {
				var m *Metric
				for i := range metrics {
					if metrics[i].Name == name {
						m = metrics[i]
						break
					}
				}
				if m == nil {
					continue
				}
				pairs = prometheus.GenTimelines(pairs, options.StartTs, options.EndTs, options.StepSec)
				for _, p := range pairs {
					result := make([]interface{}, 0, 2)
					result = append(result, p.Timestamp.Unix(), round(float64(p.Value)))
					m.MemUsage.Samples = append(m.MemUsage.Samples, result)
				}
				if len(pairs) > 0 {
					m.MemUsage.Current = round(float64(pairs[len(pairs)-1].Value))
				}
			}
			return nil
		}, nil
	}

}

func roundCpu(value float64) float64 {
	if value >= pro.DataRoundSignificant {
		return math.Floor(value*100+0.5) / 100
	} else {
		return common.FloatRound(value*1000, 2)
	}
}

func roundCpuMeasure(value float64) CPUMeasurement {
	cpu := CPUMeasurement{Unit: CPUUnitCore, Value: 0}
	if value == 0 {
		return cpu
	}
	if value >= 1 {
		cpu.Value = common.FloatRound(value, 2)
		cpu.Unit = CPUUnitCore
	} else {
		cpu.Value = common.FloatRound(value*1000, 2)
		cpu.Unit = CPUUnitMillionsCore
	}
	return cpu
}

func round(value float64) float64 {
	return math.Floor(value*100+0.5) / 100
}
