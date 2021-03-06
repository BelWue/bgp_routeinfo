package routeinfo

import (
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
)

var rs RouteInfoServer
var router *Router

const addressCacheSize = 10000

func init() {
	config, err := ioutil.ReadFile("config.yml")
	if err != nil {
		log.Printf("[error] reading config file: %s", err)
		return
	}
	err = yaml.Unmarshal(config, &rs)
	if err != nil {
		log.Fatalf("[error] Error parsing configuration YAML: %v", err)
	}
	rs.Init()
	for _, value := range rs.Routers {
		// simply use whatever the first router is
		router = value
		break
	}

	// wait for sessions to be established and in sync
	time.Sleep(10 * time.Second)
	for {
		_, ready := router.Status()
		if ready == true {
			break
		} else {
			time.Sleep(3 * time.Second)
		}
	}
	// for good measure
	time.Sleep(10 * time.Second)
}

func BenchmarkLookupIPv4Random(b *testing.B) {
	// no logs to slow us down
	log.SetOutput(ioutil.Discard)
	os.Stdout, _ = os.Open(os.DevNull)
	addresses := make([]string, addressCacheSize)
	for n := 0; n < addressCacheSize; n++ {
		addresses[n] = strconv.Itoa(rand.Intn(255)) + "." + strconv.Itoa(rand.Intn(255)) + "." + strconv.Itoa(rand.Intn(255)) + "." + strconv.Itoa(rand.Intn(255))
	}

	// let's goooo
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		router.Lookup(addresses[n%addressCacheSize])
	}
}

func BenchmarkLookupIPv4Static(b *testing.B) {
	// no logs to slow us down
	log.SetOutput(ioutil.Discard)
	os.Stdout, _ = os.Open(os.DevNull)
	addresses := make([]string, addressCacheSize)
	for n := 0; n < addressCacheSize; n++ {
		addresses[n] = "1.1.1.1"
	}

	// let's goooo
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		router.Lookup(addresses[n%addressCacheSize])
	}
}

func BenchmarkEstablished(b *testing.B) {
	// no logs to slow us down
	log.SetOutput(ioutil.Discard)
	os.Stdout, _ = os.Open(os.DevNull)
	for n := 0; n < b.N; n++ {
		router.Established()
	}
}

func BenchmarkStatus(b *testing.B) {
	// no logs to slow us down
	log.SetOutput(ioutil.Discard)
	os.Stdout, _ = os.Open(os.DevNull)
	for n := 0; n < b.N; n++ {
		router.Status()
	}
}
