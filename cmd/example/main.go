package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/BelWue/bgp_routeinfo/routeinfo"
	"gopkg.in/yaml.v2"
)

func main() {
	configfile := flag.String("c", "config.yml", "location of the config file in yml format")
	flag.Parse()

	config, err := ioutil.ReadFile(*configfile)
	if err != nil {
		log.Printf("[error] reading config file: %s", err)
		return
	}

	var rs routeinfo.RouteInfoServer
	err = yaml.Unmarshal(config, &rs)
	if err != nil {
		log.Fatalf("[error] Error parsing configuration YAML: %v", err)
	}

	rs.Init() // try to establish all sessions

	rs.Routers["router-1"].WaitForEOR() // block until ready

	// try a bunch of stuff
	fmt.Println("Lookup 1.0.128.1:")
	result := rs.Routers["router-1"].Lookup("1.0.128.1")
	fmt.Printf("  %+v\n", result)
	fmt.Println("Lookup 8.8.8.8/32:")
	result = rs.Routers["router-1"].Lookup("8.8.8.8/32")
	fmt.Printf("  %+v\n", result)
	fmt.Println("Lookup 2001:4860:4860::8844:")
	result = rs.Routers["router-1"].Lookup("2001:4860:4860::8844")
	fmt.Printf("  %+v\n", result)
	fmt.Println("LookupShorter 8.7.235.0/23:")
	result = rs.Routers["router-1"].LookupShorter("8.7.235.0/23")
	fmt.Printf("  %+v\n", result)
	fmt.Println("LookupShorter 8.8.8.8:")
	result = rs.Routers["router-1"].LookupShorter("8.8.8.8")
	fmt.Printf("  %+v\n", result)
}
