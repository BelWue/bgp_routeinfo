package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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
	flag.Parse()

	config, err := ioutil.ReadFile(*configfile)
	if err != nil {
		log.Printf("[error] reading config file: %s", err)
		return
	}

	err = yaml.Unmarshal(config, &rs)
	if err != nil {
		log.Fatalf("[error] Error parsing configuration YAML: %v", err)
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
	http.ListenAndServe(":3000", nil)
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
		pr.Paths = router.Lookup(qPrefix)
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
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	writer.Write(body)
}
