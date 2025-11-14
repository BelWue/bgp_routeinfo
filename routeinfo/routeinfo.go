package routeinfo

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/BelWue/bgp_routeinfo/log"
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

type Router struct {
	Name                     string   `yaml:"name"`
	Asn                      uint32   `yaml:"asn"`
	Neighbors                []string `yaml:"neighbors"`
	neighborSessionState     map[string]bgp.FSMState
	neighborSessionStateLock sync.Mutex
	GobgpServer              *server.BgpServer
	Logger                   log.RouteinfoLogger
}

func (r *Router) Connect() {
	for _, addr := range r.Neighbors {
		// determine AFI
		var parsed net.IP
		var afi api.Family_Afi
		if parsed = net.ParseIP(addr); parsed != nil {
		} else {
			r.Logger.GetApplicationLogger().Errorf("Invalid address: %s", addr)
			continue
		}
		if addr4 := parsed.To4(); addr4 != nil {
			afi = api.Family_AFI_IP
		} else {
			afi = api.Family_AFI_IP6
		}

		if err := r.GobgpServer.AddPeer(context.Background(), GenerateAddPeerRequest(r, addr, afi)); err != nil {
			r.Logger.GetApplicationLogger().Fatalf("Failed to add peer %s for router %s due to %v", parsed.String(), r.Name, err)
		}
	}
}

func GenerateAddPeerRequest(r *Router, addr string, afi api.Family_Afi) *api.AddPeerRequest {
	return &api.AddPeerRequest{
		Peer: &api.Peer{
			Conf: &api.PeerConf{
				NeighborAddress: addr,
				PeerAsn:         r.Asn,
			},
			// define the AFI manually to enable Add-Paths
			AfiSafis: []*api.AfiSafi{
				{
					Config: &api.AfiSafiConfig{
						Family: &api.Family{
							Afi:  afi,
							Safi: api.Family_SAFI_UNICAST,
						},
						Enabled: true,
					},
					AddPaths: &api.AddPaths{
						Config: &api.AddPathsConfig{
							Receive: true,
						},
					},
				},
			},
		},
	}
}

func (r *Router) LookupShorter(address string) []RouteInfo {
	if parsed := net.ParseIP(address); parsed != nil {
		if addr4 := parsed.To4(); addr4 != nil {
			address += "/32"
		} else {
			address += "/128"
		}
	}
	return r.lookup(address, apiutil.LOOKUP_SHORTER)
}

func (r *Router) LookupLonger(address string) []RouteInfo {
	if parsed := net.ParseIP(address); parsed != nil {
		if addr4 := parsed.To4(); addr4 != nil {
			address += "/32"
		} else {
			address += "/128"
		}
	}
	return r.lookup(address, apiutil.LOOKUP_SHORTER)
}

func (r *Router) Lookup(address string) []RouteInfo {
	return r.lookup(address, apiutil.LOOKUP_EXACT)
}

func (r *Router) lookup(address string, lookupType apiutil.LookupOption) []RouteInfo {
	// determine AFI
	var (
		parsed net.IP
		family bgp.Family
		err    error
		// subnet *net.IPNet
	)

	parsed = net.ParseIP(address)
	if parsed == nil {
		parsed, _, err = net.ParseCIDR(address)
		if err != nil {
			r.Logger.GetApplicationLogger().Warnf("Invalid address: %s: %v", address, err)
			return nil
		} else if parsed == nil {
			r.Logger.GetApplicationLogger().Warnf("Invalid address: %s", address)
			return nil
		}
	}

	if addr4 := parsed.To4(); addr4 != nil {
		family = bgp.RF_IPv4_UC
		// if subnet == nil {
		// 	address += "/32"
		// }
	} else {
		family = bgp.RF_IPv6_UC
		// if subnet == nil {
		// 	address += "/128"
		// }
	}

	// build request
	prefixIn := &apiutil.LookupPrefix{
		Prefix:       address,
		LookupOption: lookupType,
	}

	// get answer
	var (
		pr *bgp.NLRI
		pa *[]*apiutil.Path
	)
	err = r.GobgpServer.ListPath(apiutil.ListPathRequest{
		TableType: api.TableType_TABLE_TYPE_GLOBAL,
		Family:    family,
		Prefixes:  []*apiutil.LookupPrefix{prefixIn},
	}, func(prefix bgp.NLRI, paths []*apiutil.Path) {
		//debug
		r.Logger.GetApplicationLogger().Info(prefix.String())
		for _, p := range paths {
			r.Logger.GetApplicationLogger().Debug("Returned path: peer_asn = " + strconv.FormatUint(uint64(p.PeerASN), 10) + ", peer_address: " + p.PeerAddress.String() + ", age: " + strconv.FormatInt(p.Age, 10) + ", best: " + strconv.FormatBool(p.Best))
		}
		pr = &prefix
		pa = &paths
	})
	if err != nil {
		r.Logger.GetApplicationLogger().Errorf("failed to list path due to %v", err)
	}

	if err != nil {
		r.Logger.GetApplicationLogger().Errorf("Failed listing path due to %v", err)
	}

	// no answer here
	pre := ""
	if pr == nil {
		r.Logger.GetApplicationLogger().Warnf("No prefix returned for %s.", address)
	} else {
		pre = (*pr).String()
	}
	if pa == nil {
		r.Logger.GetApplicationLogger().Warnf("No destination returned for %s.", address)
		return nil
	}

	// generate a result per path returned
	var results []RouteInfo
	for _, path := range *pa {
		var (
			nexthop             *netip.Addr
			mpReach             *bgp.PathAttributeMpReachNLRI
			asPath              *[]bgp.AsPathParamInterface
			communities         *[]uint32
			origin              *uint8
			multiExitDisc       *uint32
			localPref           *uint32
			largeCommunities    *[]*bgp.LargeCommunity
			extendedCommunities *[]bgp.ExtendedCommunityInterface
			aspathNbrs          []uint32
			communityNames      []string
			largecommunityNames []string

			nexthopString string
		)

		for _, a := range path.Attrs {
			switch a.GetType() {
			case bgp.BGP_ATTR_TYPE_NEXT_HOP:
				nexthop = &a.(*bgp.PathAttributeNextHop).Value
			case bgp.BGP_ATTR_TYPE_MP_REACH_NLRI:
				mpReach = a.(*bgp.PathAttributeMpReachNLRI)
			case bgp.BGP_ATTR_TYPE_AS_PATH:
				asPath = &a.(*bgp.PathAttributeAsPath).Value
			case bgp.BGP_ATTR_TYPE_COMMUNITIES:
				communities = &a.(*bgp.PathAttributeCommunities).Value
			case bgp.BGP_ATTR_TYPE_ORIGIN:
				origin = &a.(*bgp.PathAttributeOrigin).Value
			case bgp.BGP_ATTR_TYPE_MULTI_EXIT_DISC:
				multiExitDisc = &a.(*bgp.PathAttributeMultiExitDisc).Value
			case bgp.BGP_ATTR_TYPE_LOCAL_PREF:
				localPref = &a.(*bgp.PathAttributeLocalPref).Value
			case bgp.BGP_ATTR_TYPE_LARGE_COMMUNITY:
				largeCommunities = &a.(*bgp.PathAttributeLargeCommunities).Values
			case bgp.BGP_ATTR_TYPE_EXTENDED_COMMUNITIES:
				extendedCommunities = &a.(*bgp.PathAttributeExtendedCommunities).Value
			}
		}

		if nexthop != nil {
			nexthopString = nexthop.String()
		} else if mpReach != nil {
			nexthopString = mpReach.Nexthop.String()
		} else {
			nexthopString = "N/A"
		}

		// decode aspath
		if asPath != nil {
			for _, segment := range *asPath {
				aspathNbrs = append(aspathNbrs, segment.GetAS()...)
			}
		}

		// decode communities
		if communities != nil {
			for _, community := range *communities {
				front := community >> 16
				back := community & 0xffff
				communityNames = append(communityNames, fmt.Sprintf("%d:%d", front, back))
			}
		}

		// decode large communities
		if largeCommunities != nil {
			for _, community := range *largeCommunities {
				largecommunityNames = append(
					largecommunityNames,
					community.String(),
				)
			}
		}

		// partly decode extended communities
		valid := bgp.VALIDATION_STATE_NOT_FOUND
		if extendedCommunities != nil {
			for _, ec := range *extendedCommunities {
				if val, ok := ec.(*bgp.ValidationExtended); ok {
					valid = val.State
					break
				}
			}
		}

		var originValue = OriginValue(255)
		if origin != nil {
			originValue = OriginValue(*origin)
		}

		var originAS uint32
		if len(aspathNbrs) > 0 {
			originAS = aspathNbrs[len(aspathNbrs)-1]
		} else {
			originAS = 0
		}

		var (
			localPrefResult uint32
			med             uint32
		)
		if localPref != nil {
			localPrefResult = *localPref
		}
		if multiExitDisc != nil {
			med = *multiExitDisc
		}

		result := RouteInfo{
			AsPath:           aspathNbrs,
			Best:             path.Best,
			Communities:      communityNames,
			LargeCommunities: largecommunityNames,
			LocalPref:        localPrefResult,
			Med:              med,
			NextHop:          nexthopString,
			OriginAs:         originAS,
			Origin:           originValue,
			Peer:             path.PeerAddress.String(),
			Prefix:           pre,
			Timestamp:        time.Unix(path.Age, 0),
			Validation:       valid,
		}
		results = append(results, result)
	}
	return results
}

type OriginValue uint8

const (
	IGP OriginValue = iota
	EGP
	Incomplete
)

func (v OriginValue) String() string {
	switch v {
	case IGP:
		return "IGP"
	case EGP:
		return "EGP"
	case Incomplete:
		return "Incomplete"
	default:
		return "Unknown"
	}
}

type RouteInfo struct {
	AsPath           []uint32            `json:"aspath"`
	Best             bool                `json:"best"`
	Communities      []string            `json:"communities"`
	LargeCommunities []string            `json:"largecommunities"`
	LocalPref        uint32              `json:"localpref"`
	Med              uint32              `json:"med"`
	NextHop          string              `json:"nexthop"`
	OriginAs         uint32              `json:"originas"`
	Origin           OriginValue         `json:"origin"`
	Peer             string              `json:"peer"`
	Prefix           string              `json:"prefix"`
	Timestamp        time.Time           `json:"timestamp"`
	Validation       bgp.ValidationState `json:"validation"`
}

func (router *Router) Status() (bool, bool) {
	var ready = true
	for _, address := range router.Neighbors {
		router.GobgpServer.ListPeer(context.Background(), &api.ListPeerRequest{Address: address}, func(p *api.Peer) {
			for _, a := range p.AfiSafis {
				s := a.MpGracefulRestart.State
				ready = ready && s.EndOfRibReceived
			}
		})
	}
	connected := router.Established()
	return connected, ready
}

func (router *Router) Established() bool {
	router.neighborSessionStateLock.Lock()
	defer router.neighborSessionStateLock.Unlock()
	for _, state := range router.neighborSessionState {
		if state != bgp.BGP_FSM_ESTABLISHED {
			return false
		}
	}
	return true
}

func (router *Router) WaitForEOR() {
	// TODO check for EOR separately
	var ready bool
	for !ready && router != nil {
		time.Sleep(time.Second * 1)
		_, ready = router.Status()
		if !ready {
			router.Logger.GetApplicationLogger().Infof("Waiting for connection to router %s", router.Name)
		}
	}
}

func (rs *RouteInfoServer) SetLogLevel(logLevel *string) {
	rs.Logger.SetLogLevel(logLevel)
}
