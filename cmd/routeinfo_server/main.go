package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"

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
	Connected bool   `json:"connected"`
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

	http.HandleFunc("/prefix", prefix)
	http.HandleFunc("/status", status)
	http.ListenAndServe(":3000", nil)
}

func status(writer http.ResponseWriter, request *http.Request) {
	var response StatusResponse
	for name, router := range rs.Routers {
		var rStatus RouterStatus
		rStatus.Router = name
        rStatus.Connected, rStatus.Ready = router.Status()
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

	qRouter := request.URL.Query().Get("router")
	var router *routeinfo.Router
	var ok bool
	if router, ok = rs.Routers[qRouter]; !ok {
		response.Errors = append(response.Errors, "No such router.")
	} else {
		// TODO remove debug
		response.Errors = append(response.Errors, router.Name)
	}

	qPrefix := request.URL.Query().Get("prefix")
	// TODO: properly validate input
	//var valid = true
	//if !valid {
	//	errors.append(errors, "No such .")
	//}

	// TODO this should be able to loop over multiple or all routers
	var pr PrefixResult
	pr.Router = qRouter // FIXME TODO
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
