package routeinfo

import (
	"context"
	"sync"
	"time"

	"github.com/BelWue/bgp_routeinfo/log"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/osrg/gobgp/v4/api"
	"github.com/osrg/gobgp/v4/pkg/apiutil"
	"github.com/osrg/gobgp/v4/pkg/metrics"
	"github.com/osrg/gobgp/v4/pkg/packet/bgp"
	"github.com/osrg/gobgp/v4/pkg/server"
)

type RouteInfoServer struct {
	Asn      uint32             `yaml:"asn"`
	RouterId string             `yaml:"routerid"`
	Routers  map[string]*Router `yaml:"routers"`
	Logger   log.RouteinfoLogger
}

func (rs RouteInfoServer) EnableExtraFields() {
	for _, router := range rs.Routers {
		router.DecodeExtraFields = true
	}
}

func (rs *RouteInfoServer) InitLogger(logLevel *string) {
	rs.Logger = &log.DefaultRouteInfoLogger{}
	if logLevel != nil {
		rs.SetLogLevel(logLevel)
	}
	for _, router := range rs.Routers {
		router.Logger = rs.Logger
	}
}

func (rs *RouteInfoServer) getBgpInstance(router *Router) *server.BgpServer {
	fsmTimingCollector := metrics.NewFSMTimingsCollector()
	bgpServer := server.NewBgpServer(
		server.LoggerOption(rs.Logger.GetBgpLogger()),
		server.TimingHookOption(fsmTimingCollector))
	go bgpServer.Serve()

	if rs.RouterId == "" {
		rs.RouterId = "255.255.255.255" // we should be able to get away with this
	}

	// global configuration
	if err := bgpServer.StartBgp(context.Background(), &api.StartBgpRequest{
		Global: &api.Global{
			Asn:        rs.Asn,
			RouterId:   rs.RouterId,
			ListenPort: -1, // gobgp won't listen on tcp:179
		},
	}); err != nil {
		rs.Logger.GetApplicationLogger().Fatalf("Failed to start BGP due to: %e", err)
	}

	callbacks := server.WatchEventMessageCallbacks{}
	callbacks.OnPeerUpdate = func(r *apiutil.WatchEventMessage_PeerEvent, t time.Time) {
		peer := r.Peer.State
		if peer.NeighborAddress.BitLen() == 0 {
			// I don't know why this happens, but I don't want it logged.
			return
		}
		router.neighborSessionStateLock.Lock()
		router.neighborSessionState[peer.NeighborAddress.String()] = peer.SessionState
		router.neighborSessionStateLock.Unlock()
		if peer.SessionState == bgp.BGP_FSM_ESTABLISHED {
			rs.Logger.GetApplicationLogger().Infof("Peer %d/%s is in FSM state '%s' (admin state = '%s')", peer.PeerASN, peer.NeighborAddress, peer.SessionState, peer.AdminState.String())
		} else {
			rs.Logger.GetApplicationLogger().Debugf("Peer %d/%s is in FSM state '%s' (admin state = '%s')", peer.PeerASN, peer.NeighborAddress, peer.SessionState, peer.AdminState.String())
		}
	}

	callbacks.OnPathUpdate = func(p []*apiutil.Path, t time.Time) {
		rs.Logger.GetApplicationLogger().Infof("OnPathUpdate: %v", p)
	}
	callbacks.OnBestPath = func(p []*apiutil.Path, t time.Time) {
		rs.Logger.GetApplicationLogger().Infof("OnBestPath: %v", p)
	}
	callbacks.OnPathEor = func(p *apiutil.Path, t time.Time) {
		rs.Logger.GetApplicationLogger().Infof("OnPathEor: %v", p)
	}

	watchOption := server.WatchPeer()
	err := bgpServer.WatchEvent(context.Background(), callbacks, watchOption)
	if err != nil {
		rs.Logger.GetApplicationLogger().Errorf("Failed to create bgp session %v", err)
	}

	return bgpServer
}

func (rs *RouteInfoServer) Init() {
	for name, router := range rs.Routers {
		if router.CacheTTL == 0 {
			router.CacheTTL = 1 * time.Second
		} else if router.CacheTTL < 0 {
			router.CacheTTL = 0
		}
		if router.CacheTTL > 0 {
			size := router.CacheSize
			if size <= 0 {
				size = 8192
			}
			router.cache = lru.NewLRU[cacheKey, []RouteInfo](size, nil, router.CacheTTL)
		}

		router.Logger = rs.Logger
		if len(router.Neighbors) == 0 {
			rs.Logger.GetApplicationLogger().Fatalf("unconfigured router %s\n", name)
		}
		if router.Asn == 0 {
			router.Asn = rs.Asn
		}
		if router.GobgpServer == nil {
			router.GobgpServer = rs.getBgpInstance(router)
		}
		if router.neighborSessionState == nil {
			router.neighborSessionState = make(map[string]bgp.FSMState)
		}
		router.Connect()
	}
}

func (rs *RouteInfoServer) Stop() {
	var wg sync.WaitGroup
	for _, router := range rs.Routers {
		wg.Add(1)
		go func(myrouter *Router) {
			defer wg.Done()
			myrouter.GobgpServer.Stop()
		}(router)
	}
	wg.Wait()
}

func (rs *RouteInfoServer) SetLogLevel(logLevel *string) {
	rs.Logger.SetLogLevel(logLevel)
}
