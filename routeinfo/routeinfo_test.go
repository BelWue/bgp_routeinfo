package routeinfo

import (
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"gopkg.in/yaml.v2"
)

var rs RouteInfoServer
var router *Router

const addressCacheSize = 10000

func init() {
	config, err := os.ReadFile("config.yml")
	if err != nil {
		log.Error().Err(err).Msg("Failed reading config file")
		return
	}
	err = yaml.Unmarshal(config, &rs)
	if err != nil {
		log.Fatal().Err(err).Msg("Error parsing configuration YAML")
	}
	rs.Init()
	for _, value := range rs.Routers {
		// simply use whatever the first router is
		router = value
		break
	}

	if router == nil {
		log.Warn().Msg("No router initialized. Please check your config")
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
	zerolog.SetGlobalLevel(zerolog.Disabled)
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
	zerolog.SetGlobalLevel(zerolog.Disabled)
	zerolog.SetGlobalLevel(zerolog.Disabled)
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
	zerolog.SetGlobalLevel(zerolog.Disabled)
	os.Stdout, _ = os.Open(os.DevNull)
	for n := 0; n < b.N; n++ {
		router.Established()
	}
}

func BenchmarkStatus(b *testing.B) {
	// no logs to slow us down
	zerolog.SetGlobalLevel(zerolog.Disabled)
	os.Stdout, _ = os.Open(os.DevNull)
	for n := 0; n < b.N; n++ {
		router.Status()
	}
}
