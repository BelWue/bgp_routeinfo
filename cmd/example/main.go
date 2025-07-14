package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/BelWue/bgp_routeinfo/log"
	"github.com/BelWue/bgp_routeinfo/routeinfo"
	zerolog "github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

func main() {
	configfile := flag.String("c", "config.yml", "location of the config file in yml format")
	flag.Parse()

	config, err := os.ReadFile(*configfile)
	if err != nil {
		zerolog.Error().Err(err).Msgf("Failed reading config file: %s", *configfile)
		return
	}

	var rs routeinfo.RouteInfoServer
	err = yaml.Unmarshal(config, &rs)
	if err != nil {
		zerolog.Fatal().Err(err).Msg("Error parsing configuration YAML")
	}

	rs.Init() // try to establish all sessions

	rs.Routers["router-1"].WaitForEOR() // block until ready

	logger := log.ApplicationLoggerFromZerolog(&zerolog.Logger)

	// try a bunch of stuff
	fmt.Println("Lookup 1.0.128.1:")
	result := rs.Routers["router-1"].Lookup("1.0.128.1", logger)
	fmt.Printf("  %+v\n", result)
	fmt.Println("Lookup 8.8.8.8/32:")
	result = rs.Routers["router-1"].Lookup("8.8.8.8/32", logger)
	fmt.Printf("  %+v\n", result)
	fmt.Println("Lookup 2001:4860:4860::8844:")
	result = rs.Routers["router-1"].Lookup("2001:4860:4860::8844", logger)
	fmt.Printf("  %+v\n", result)
	fmt.Println("LookupShorter 8.7.235.0/23:")
	result = rs.Routers["router-1"].LookupShorter("8.7.235.0/23", logger)
	fmt.Printf("  %+v\n", result)
	fmt.Println("LookupShorter 8.8.8.8:")
	result = rs.Routers["router-1"].LookupShorter("8.8.8.8", logger)
	fmt.Printf("  %+v\n", result)
}
