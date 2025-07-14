package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	applog "github.com/BelWue/bgp_routeinfo/log"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/BelWue/bgp_routeinfo/routeinfo"
	"gopkg.in/yaml.v2"
)

type PrefixResult struct {
	Router string                `json:"router"`
	Prefix string                `json:"prefix"`
	Paths  []routeinfo.RouteInfo `json:"paths"`
}

type RouterStatus struct {
	Router string `json:"router"`
	Ready  bool   `json:"ready"`
}

type StatusResponse struct {
	Errors  []string       `json:"errors"`
	Results []RouterStatus `json:"results"`
}

type PrefixResponse struct {
	Errors  []string       `json:"errors"`
	Results []PrefixResult `json:"results"`
}

var rs routeinfo.RouteInfoServer

func main() {

	configfile := flag.String("c", "config.yml", "location of the config file in yml format")
	jsonLogging := flag.Bool("j", false, "Json log")
	endpoint := flag.String("e", ":3000", "Endpoint the service should listen/serve on")
	logLevel := flag.String("l", "info", "Loglevel: one of 'debug', 'info', 'warning' or 'error'")
	flag.Parse()

	if !*jsonLogging {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.DateTime})
	}
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(zerologLogLevel(logLevel))

	config, err := os.ReadFile(*configfile)
	if err != nil {
		log.Error().Err(err).Msgf("reading config file: %s", *configfile)
		return
	}

	err = yaml.Unmarshal(config, &rs)
	if err != nil {
		log.Fatal().Err(err).Msg("Error parsing configuration YAML")
	}

	rs.Init() // try to establish all sessions

	// clean shutdown on ^C
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	go func() {
		<-sigc
		rs.Stop()
		os.Exit(0)
	}()

	http.HandleFunc("/prefix", prefix)
	http.HandleFunc("/status", status)
	err = http.ListenAndServe(*endpoint, nil)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to listen on %s", *endpoint)
	}
}

func status(writer http.ResponseWriter, request *http.Request) {
	var response StatusResponse
	for name, router := range rs.Routers {
		var rStatus RouterStatus
		rStatus.Router = name
		// TODO also check if EOR was seen
		rStatus.Ready = router.Established()
		response.Results = append(response.Results, rStatus)
	}
	body, err := json.Marshal(response)
	if err != nil {
		// can't really add error strings to the body here anymore...
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	writer.Write(body)
}

func prefix(writer http.ResponseWriter, request *http.Request) {
	var response PrefixResponse

	qRouters := request.URL.Query()["router"]
	routers := make(map[string]*routeinfo.Router)
	if (len(qRouters) == 1 && len(qRouters[0]) > 0) || len(qRouters) > 1 {
		for _, qRouter := range qRouters {
			if router, ok := rs.Routers[qRouter]; ok {
				routers[qRouter] = router
			} else {
				response.Errors = append(response.Errors, "Router not found.")
			}
		}
	} else {
		// no filter for router name, so use all routers
		routers = rs.Routers
	}

	qPrefix := request.URL.Query().Get("prefix")
	// TODO: properly validate input
	//var valid = true
	//if !valid {
	//	errors.append(errors, "No such .")
	//}

	for routerName, router := range routers {
		var pr PrefixResult
		pr.Router = routerName
		pr.Paths = router.Lookup(qPrefix, applog.ApplicationLoggerFromZerolog(&log.Logger))
		if len(pr.Paths) > 0 {
			pr.Prefix = pr.Paths[0].Prefix
			for _, path := range pr.Paths[1:] {
				if path.Prefix != pr.Prefix {
					response.Errors = append(response.Errors, "RIB returned multiple paths with different prefixes.")
					break
				}
			}
			response.Results = append(response.Results, pr)
		}
	}

	body, err := json.Marshal(response)
	if err != nil {
		// can't really add error strings to the body here anymore...
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		log.Error().Err(err).Msg("Http Request error")
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	writer.Write(body)
}

func zerologLogLevel(logLevel *string) zerolog.Level {
	if logLevel != nil && *logLevel != "" {
		switch *logLevel {
		case "trace":
			log.Info().Msg("Using log level 'trace'")
			return zerolog.TraceLevel
		case "debug":
			log.Info().Msg("Using log level 'debug'")
			return zerolog.DebugLevel
		case "info":
			log.Info().Msg("Using log level 'info'")
			return zerolog.InfoLevel
		case "warning":
			return zerolog.WarnLevel
		case "error":
			return zerolog.ErrorLevel
		case "fatal":
			return zerolog.FatalLevel
		case "panic":
			return zerolog.PanicLevel
		default:
			log.Warn().Msgf("Unknown log level '%s' using default 'info'", *logLevel)
		}
	} else {
		log.Info().Msg("Using default log level 'info'")
	}

	return zerolog.InfoLevel
}
