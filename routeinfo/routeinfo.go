package routeinfo

import (
	"context"
	"fmt"
	"net"
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
	rs.SetLogLevel(logLevel)
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
		if len(peer.NeighborAddress) == 0 {
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
		router.Connect(rs.Logger.GetApplicationLogger())
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
}

func (r *Router) Connect(log log.ApplicationLogger) {
	for _, addr := range r.Neighbors {
		// determine AFI
		var parsed net.IP
		var afi api.Family_Afi
		if parsed = net.ParseIP(addr); parsed != nil {
		} else {
			log.Errorf("Invalid address: %s", addr)
			continue
		}
		if addr4 := parsed.To4(); addr4 != nil {
			afi = api.Family_AFI_IP
		} else {
			afi = api.Family_AFI_IP6
		}

		if err := r.GobgpServer.AddPeer(context.Background(), GenerateAddPeerRequest(r, addr, afi)); err != nil {
			log.Fatalf("Failed to add peer %s for router %s due to %v", parsed.String(), r.Name, err)
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

func (r *Router) LookupShorter(address string, log log.ApplicationLogger) []RouteInfo {
	if parsed := net.ParseIP(address); parsed != nil {
		if addr4 := parsed.To4(); addr4 != nil {
			address += "/32"
		} else {
			address += "/128"
		}
	}
	return r.lookup(address, api.TableLookupPrefix_TYPE_SHORTER, log)
}

func (r *Router) LookupLonger(address string, log log.ApplicationLogger) []RouteInfo {
	if parsed := net.ParseIP(address); parsed != nil {
		if addr4 := parsed.To4(); addr4 != nil {
			address += "/32"
		} else {
			address += "/128"
		}
	}
	return r.lookup(address, api.TableLookupPrefix_TYPE_LONGER, log)
}

func (r *Router) Lookup(address string, logger log.ApplicationLogger) []RouteInfo {
	//0 Gets parsed in https://github.com/osrg/gobgp/blob/1e52815dc83b975a10819e30df65bc6fa2f96baf/internal/pkg/table/table.go#L40
	return r.lookup(address, 0, logger)
}

func (r *Router) lookup(address string, lookupType api.TableLookupPrefix_Type, log log.ApplicationLogger) []RouteInfo {
	// determine AFI
	var (
		parsed net.IP
		afi    api.Family_Afi
		err    error
		// subnet *net.IPNet
	)

	parsed = net.ParseIP(address)
	if parsed == nil {
		parsed, _, err = net.ParseCIDR(address)
		if err != nil {
			log.Warnf("Invalid address: %s: %v", address, err)
			return nil
		} else if parsed == nil {
			log.Warnf("Invalid address: %s", address)
			return nil
		}
	}

	if addr4 := parsed.To4(); addr4 != nil {
		afi = api.Family_AFI_IP
		// if subnet == nil {
		// 	address += "/32"
		// }
	} else {
		afi = api.Family_AFI_IP6
		// if subnet == nil {
		// 	address += "/128"
		// }
	}

	// build request
	family := &api.Family{Afi: afi, Safi: api.Family_SAFI_UNICAST}
	prefix := &api.TableLookupPrefix{Prefix: address, Type: lookupType}
	req := &api.ListPathRequest{
		TableType: api.TableType_TABLE_TYPE_GLOBAL,
		Family:    family,
		Prefixes:  []*api.TableLookupPrefix{prefix},
	}

	log.Infof("Request %v for router %v", req, r)

	// get answer
	var destination *api.Destination
	err = r.GobgpServer.ListPath(context.Background(), req, func(d *api.Destination) {
		log.Info("return function called")
		destination = d
	})

	if err != nil {
		log.Errorf("Failed listing path due to %v", err)
	}

	// no answer here
	if destination == nil {
		log.Warnf("No destination returned for %s.", address)
		return nil
	}

	// generate a result per path returned
	var results []RouteInfo
	for _, path := range destination.Paths {
		var prefix = path.GetNlri().GetPrefix()
		if prefix == nil {
			log.Warnf("No prefix found for this destination path: %+v\n", path)
			prefix = &api.IPAddressPrefix{
				PrefixLen: 7,
				Prefix:    "unknown",
			}
		}

		var (
			nexthopSting        string
			nexthop             *api.NextHopAttribute
			mpReach             *api.MpReachNLRIAttribute
			asPath              *api.AsPathAttribute
			community           *api.CommunitiesAttribute
			origin              *api.OriginAttribute
			multiExitDisc       *api.MultiExitDiscAttribute
			localPref           *api.LocalPrefAttribute
			largeCommunities    *api.LargeCommunitiesAttribute
			extendedCommunities *api.ExtendedCommunitiesAttribute
			aspathNbrs          []uint32
			communityNames      []string
			largecommunityNames []string
		)

		for _, pattrn := range path.Pattrs {
			switch a := pattrn.Attr.(type) {
			case *api.Attribute_NextHop:
				nexthop = a.NextHop
			case *api.Attribute_MpReach:
				mpReach = a.MpReach
			case *api.Attribute_AsPath:
				asPath = a.AsPath
			case *api.Attribute_Communities:
				community = a.Communities
			case *api.Attribute_Origin:
				origin = a.Origin
			case *api.Attribute_MultiExitDisc:
				multiExitDisc = a.MultiExitDisc
			case *api.Attribute_LocalPref:
				localPref = a.LocalPref
			case *api.Attribute_LargeCommunities:
				largeCommunities = a.LargeCommunities
			case *api.Attribute_ExtendedCommunities:
				extendedCommunities = a.ExtendedCommunities
			}
		}

		nextHopSet := false
		if nexthop != nil {
			if nexthop.NextHop != "" {
				nexthopSting = nexthop.NextHop
				nextHopSet = true
			}
		} else if mpReach != nil {
			if len(mpReach.NextHops) > 0 {
				nexthopSting = mpReach.NextHops[0]
				nextHopSet = true
			}
		}
		if !nextHopSet {
			nexthopSting = "N/A"
		}

		// decode aspath
		if asPath != nil {
			for _, segment := range asPath.Segments {
				aspathNbrs = append(aspathNbrs, segment.Numbers...)
			}
		}

		// decode communities
		if community != nil {
			for _, community := range community.Communities {
				front := community >> 16
				back := community & 0xffff
				communityNames = append(communityNames, fmt.Sprintf("%d:%d", front, back))
			}
		}

		// decode large communities
		if largeCommunities != nil {
			for _, community := range largeCommunities.Communities {
				largecommunityNames = append(largecommunityNames, fmt.Sprintf("%d:%d:%d", community.GlobalAdmin, community.LocalData1, community.LocalData2))
			}
		}

		// partly decode extended communities
		var valid = ValidationStatus(255)
		if extendedCommunities != nil {
			for _, ec := range extendedCommunities.Communities {
				validationExtended := ec.GetValidation()
				if validationExtended != nil {
					valid = ValidationStatus(validationExtended.State)
					break
				}
			}
		}

		var originValue = OriginValue(255)
		if origin != nil {
			originValue = OriginValue(origin.Origin)
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
			localPrefResult = localPref.LocalPref
		}
		if multiExitDisc != nil {
			med = multiExitDisc.Med
		}

		result := RouteInfo{
			AsPath:           aspathNbrs,
			Best:             path.Best,
			Communities:      communityNames,
			LargeCommunities: largecommunityNames,
			LocalPref:        localPrefResult,
			Med:              med,
			NextHop:          nexthopSting,
			OriginAs:         originAS,
			Origin:           originValue,
			Peer:             path.NeighborIp,
			Prefix:           fmt.Sprintf("%s/%d", prefix.Prefix, prefix.PrefixLen),
			Timestamp:        path.Age.AsTime(),
			Validation:       valid,
		}
		results = append(results, result)
	}
	return results
}

type ValidationStatus uint8

const (
	Valid ValidationStatus = iota
	NotFound
	Invalid
)

func (v ValidationStatus) String() string {
	switch v {
	case Valid:
		return "Valid"
	case NotFound:
		return "NotFound"
	case Invalid:
		return "Invalid"
	default:
		return "Unknown"
	}
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
	AsPath           []uint32         `json:"aspath"`
	Best             bool             `json:"best"`
	Communities      []string         `json:"communities"`
	LargeCommunities []string         `json:"largecommunities"`
	LocalPref        uint32           `json:"localpref"`
	Med              uint32           `json:"med"`
	NextHop          string           `json:"nexthop"`
	OriginAs         uint32           `json:"originas"`
	Origin           OriginValue      `json:"origin"`
	Peer             string           `json:"peer"`
	Prefix           string           `json:"prefix"`
	Timestamp        time.Time        `json:"timestamp"`
	Validation       ValidationStatus `json:"validation"`
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

func (router *Router) WaitForEOR(log log.ApplicationLogger) {
	// TODO check for EOR separately
	var ready bool
	for !ready && router != nil {
		time.Sleep(time.Second * 1)
		_, ready = router.Status()
		if !ready {
			log.Infof("Waiting for connection to router %s", router.Name)
		}
	}
}

func (rs *RouteInfoServer) SetLogLevel(logLevel *string) {
	rs.Logger.SetLogLevel(logLevel)
}
