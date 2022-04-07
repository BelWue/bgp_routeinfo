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

	http.HandleFunc("/lookup", lookup)
	http.HandleFunc("/list", list)
	http.ListenAndServe(":3000", nil)
}

func list(writer http.ResponseWriter, request *http.Request) {
	var rawResult []string
	for _, router := range rs.Routers {
		rawResult = append(rawResult, router.Name)
	}
	result, _ := json.Marshal(rawResult)

	writer.Header().Set("Content-Type", "application/json")
	writer.Write(result)
}

func lookup(writer http.ResponseWriter, request *http.Request) {
	qRouter := request.URL.Query().Get("router")
	var router *routeinfo.Router
	var ok bool
	if router, ok = rs.Routers[qRouter]; !ok {
		http.Error(writer, "No such router!", http.StatusInternalServerError)
		return
	}

	qPrefix := request.URL.Query().Get("prefix")
	// TODO: properly do this
	var valid = true
	if !valid {
		http.Error(writer, "Oh no!", http.StatusInternalServerError)
		return
	}

	result := router.Lookup(qPrefix)
	jResult, err := json.Marshal(result)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Write(jResult)
}
