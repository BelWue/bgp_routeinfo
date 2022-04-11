package routeinfo

import (
	"io/ioutil"
	"log"
	"testing"

	"gopkg.in/yaml.v2"
)

func BenchmarkLookup(b *testing.B) {
	config, err := ioutil.ReadFile("config.yml")
	if err != nil {
		log.Printf("[error] reading config file: %s", err)
		return
	}

	var rs RouteInfoServer
	err = yaml.Unmarshal(config, &rs)
	if err != nil {
		log.Fatalf("[error] Error parsing configuration YAML: %v", err)
	}

	for n := 0; n < b.N; n++ {
		rs.Routers["router-1"].Lookup("1.1.1.1")
	}
}
