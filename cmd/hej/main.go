package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/huhenry/hej/pkg/config"
	"github.com/huhenry/hej/pkg/log"
	"github.com/huhenry/hej/pkg/version"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	serviceName = `hej`
	HttpTimeout = 30
)

var logger = log.RegisterScope("Setup")

var metricsAddr string = ":8080"

func main() {

	v := viper.New()
	debugServerAddr := ":9999"

	var command = &cobra.Command{
		Use:   "hej",
		Short: "hej service provides an API for accessing microservice data.",
		Long:  `Hej service provides an API for accessing microservice data.`,
		RunE: func(cmd *cobra.Command, args []string) error {

			cfg, err := config.TryLoadConfigFile(v)
			if err != nil {
				return err
			}

			go func() {
				log.Logger.Debugf("Debug listening addr: %s", debugServerAddr)
				log.Logger.Fatal(http.ListenAndServe(debugServerAddr, nil))
			}()

			// logger
			log.InitFromViper(v)

			///////////////////////////////////////
			////  http service

			if !cfg.IsSet("http.http_addr") {
				logger.Errorf("http address is not in config")

				return fmt.Errorf("http address is not in config")
			}

			httptimeout := cfg.GetInt("http.http_timeout")
			if httptimeout == 0 {
				httptimeout = HttpTimeout
			}

			err = backend.Init(cfg)
			if err != nil {
				logger.Errorf("init backend client failed %s", err)
				return err
			}

			// http service init
			router.Api().ConfigDefault().
				WithManager(mgr).
				SetTimeout(time.Duration(httptimeout) * time.Second).
				InitRouter().
				Runapi(cfg.GetString("http.http_addr"))

			prometheusmetrics.RegisterInternalMetrics()

			prometheusmetrics.StartMetricsServer(metricsAddr)

			//logger.Infof("http service init ok")

			////////
			logger.Infof("          ________                                                     ")
			logger.Infof("       __/_/      |______   %s.%s is running                ", version.Get().App, version.Get().GitVersion)
			logger.Infof("      / O O O O O O O O O ...........................................  ")
			logger.Infof("                                                                       ")
			logger.Infof("      %s", time.Now().String())
			logger.Infof("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
			logger.Infof("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
			logger.Infof("      %+v", version.Get())

			////////
			// signal
			InitSignal()

			return nil
		},
	}

	config.AddFlags(
		v,
		command,
		config.AddConfigFileFlag,
		config.AddBaseFlags,
		log.AddFlags,
		clientOptions.AddFlags,
	)

	if error := command.Execute(); error != nil {
		fmt.Println(error.Error())
		os.Exit(1)
	}

}

func InitSignal() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT, syscall.SIGSTOP, syscall.SIGUSR1, syscall.SIGUSR2)
	//logger.Infof("Wait for signal.......")
	for {
		s := <-c
		logger.Infof("service[%s] get a signal %s", version.Version, s.String())
		switch s {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGSTOP, syscall.SIGINT, syscall.SIGHUP:
			GracefulQuit()
			return
		case syscall.SIGUSR2:
			//todo: Define your signal processing functions
		case syscall.SIGUSR1:
			// todo: Define signal processing functions
			//return
		default:
			return
		}
	}
}

func GracefulQuit() {
	logger.Infof("service make a graceful quit !!!!!!!!!!!!!!")
	router.Api().Shutdown() // close http service
	// close your service here

	time.Sleep(1 * time.Second)
}

func InitDependencyService(cfg *config.TpaasConfig) error {
	return iocgo.LaunchEngine(cfg)
}

func CloseDependencyService() error {
	return iocgo.StopEngine()
}

func InitPidfile(cfg *config.TpaasConfig) error {
	//pid file
	pidfile := ""
	if !cfg.IsSet("common.pid_file") {
		return nil
	} else {
		pidfile = cfg.GetString("common.pid_file")
	}
	contents, err := ioutil.ReadFile(pidfile)
	if err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
		if err != nil {
			logger.Errorf("Error reading proccess id from pidfile '%s': %s",
				pidfile, err)
			return errors.WithMessage(err, "reading proccess id from pidfile")
		}

		process, err := os.FindProcess(pid)

		// on Windows, err != nil if the process cannot be found
		if runtime.GOOS == "windows" {
			if err == nil {
				logger.Errorf("Process %d is already running.", pid)
				return fmt.Errorf("already running")
			}
		} else if process != nil {
			// err is always nil on POSIX, so we have to send the process
			// a signal to check whether it exists
			if err = process.Signal(syscall.Signal(0)); err == nil {
				logger.Errorf("Process %d is already running.", pid)
				return fmt.Errorf("already running")
			}
		}
	}
	if err = ioutil.WriteFile(pidfile, []byte(strconv.Itoa(os.Getpid())),
		0644); err != nil {

		logger.Errorf("Unable to write pidfile '%s': %s", pidfile, err)
		return err
	}
	logger.Infof("Wrote pid to pidfile '%s'", pidfile)
	return nil
}

func QuitPidFile(cfg *config.TpaasConfig) {
	pidfile := ""
	if !cfg.IsSet("common.pid_file") {
		return
	} else {
		pidfile = cfg.GetString("common.pid_file")
	}

	if err := os.Remove(pidfile); err != nil {
		logger.Errorf("Unable to remove pidfile '%s': %s", pidfile, err)
	}
	return
}
