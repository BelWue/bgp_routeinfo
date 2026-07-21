package routeinfo

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/BelWue/bgp_routeinfo/log"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/osrg/gobgp/v4/api"
	"github.com/osrg/gobgp/v4/pkg/apiutil"
	"github.com/osrg/gobgp/v4/pkg/packet/bgp"
	"github.com/osrg/gobgp/v4/pkg/server"
)

type Router struct {
	Name                     string   `yaml:"name"`
	Asn                      uint32   `yaml:"asn"`
	Neighbors                []string `yaml:"neighbors"`
	neighborSessionState     map[string]bgp.FSMState
	neighborSessionStateLock sync.Mutex
	GobgpServer              *server.BgpServer
	Logger                   log.RouteinfoLogger

	CacheTTL  time.Duration `yaml:"cache_ttl"`  // Default = 1m - Set to -1 to disable cache
	CacheSize int           `yaml:"cache_size"` // Default = 8192 entries

	DecodeExtraFields bool
	mu                sync.RWMutex
	cache             *lru.LRU[cacheKey, []RouteInfo]
}

type cacheKey struct {
	Address    string
	LookupType apiutil.LookupOption
}

func (r *Router) Connect() {
	for _, addr := range r.Neighbors {
		// determine AFI
		parsed, err := netip.ParseAddr(addr)
		if err != nil {
			r.Logger.GetApplicationLogger().Errorf("Invalid address: %s", addr)
			continue
		}

		var afi api.Family_Afi
		if parsed.Is4() || parsed.Is4In6() {
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
	address = prepareAddress(address)
	return r.lookup(address, apiutil.LOOKUP_SHORTER)
}

func (r *Router) LookupLonger(address string) []RouteInfo {
	address = prepareAddress(address)
	return r.lookup(address, apiutil.LOOKUP_LONGER)
}

func prepareAddress(address string) string {
	if addr, err := netip.ParseAddr(address); err == nil {
		if addr.Is4() || addr.Is4In6() {
			address += "/32"
		} else {
			address += "/128"
		}
	}
	return address
}

func (r *Router) Lookup(address string) []RouteInfo {
	return r.lookup(address, apiutil.LOOKUP_EXACT)
}

func (r *Router) lookup(address string, lookupType apiutil.LookupOption) []RouteInfo {
	key := cacheKey{
		Address:    address,
		LookupType: lookupType,
	}

	if cached, ok := r.getCached(key); ok {
		return cached
	}

	result := r.lookupNoCache(address, lookupType)

	r.setCached(key, result)
	return result
}

func (r *Router) lookupNoCache(address string, lookupType apiutil.LookupOption) []RouteInfo {
	var (
		family bgp.Family
		err    error
	)

	if addr, perr := netip.ParseAddr(address); perr == nil {
		family = familyFromAddr(addr)
	} else if prefix, perr := netip.ParsePrefix(address); perr == nil {
		family = familyFromAddr(prefix.Addr())
	} else {
		r.Logger.GetApplicationLogger().Warnf("Invalid address: %s", address)
		return nil
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
		for _, p := range paths {
			r.Logger.GetApplicationLogger().Debugf(
				"Returned path: peer_asn = %d, peer_address: %s, age: %d, best: %t",
				p.PeerASN, p.PeerAddress, p.Age, p.Best,
			)
		}
		pr = &prefix
		pa = &paths
	})
	if err != nil {
		r.Logger.GetApplicationLogger().Errorf("failed to list path due to %v", err)
	}

	// no answer here
	pre := ""
	if pr == nil {
		r.Logger.GetApplicationLogger().Infof("No prefix returned for %s.", address)
	} else {
		pre = (*pr).String()
	}
	if pa == nil {
		r.Logger.GetApplicationLogger().Infof("No destination returned for %s.", address)
		return nil
	}

	// generate a result per path returned
	results := make([]RouteInfo, 0, len(*pa))
	for _, path := range *pa {
		result := decodeRouteInfoMinimal(path)
		if r.DecodeExtraFields {
			decodeExtraFields(&result, path, pre)
		}
		results = append(results, result)
	}
	return results
}

func familyFromAddr(addr netip.Addr) bgp.Family {
	if addr.Is4() || addr.Is4In6() {
		return bgp.RF_IPv4_UC
	}
	return bgp.RF_IPv6_UC
}

func decodeExtraFields(result *RouteInfo, path *apiutil.Path, pre string) {
	var (
		origin           *uint8
		communities      *[]uint32
		largeCommunities *[]*bgp.LargeCommunity
	)

	for _, a := range path.Attrs {
		switch a.GetType() {
		case bgp.BGP_ATTR_TYPE_COMMUNITIES:
			communities = &a.(*bgp.PathAttributeCommunities).Value
		case bgp.BGP_ATTR_TYPE_LARGE_COMMUNITY:
			largeCommunities = &a.(*bgp.PathAttributeLargeCommunities).Values
		case bgp.BGP_ATTR_TYPE_ORIGIN:
			origin = &a.(*bgp.PathAttributeOrigin).Value
		}
	}

	result.Prefix = pre
	decodeOrigin(result, origin)
	result.Timestamp = time.Unix(path.Age, 0)
	result.Peer = path.PeerAddress.String()
	decodeCommunities(result, communities)
	decodeLargeCommunities(result, largeCommunities)
}

func decodeAsPath(target *RouteInfo, asPath *[]bgp.AsPathParamInterface) {
	var aspathNbrs []uint32
	var originAS uint32
	if asPath == nil {
		target.OriginAs = 0
		return
	}
	for _, segment := range *asPath {
		aspathNbrs = append(aspathNbrs, segment.GetAS()...)
	}
	if len(aspathNbrs) > 0 {
		originAS = aspathNbrs[len(aspathNbrs)-1]
	} else {
		originAS = 0
	}

	target.AsPath = aspathNbrs
	target.OriginAs = originAS
}

func decodeCommunities(target *RouteInfo, communities *[]uint32) {
	var communityNames []string
	if communities == nil {
		return
	}
	for _, community := range *communities {
		front := community >> 16
		back := community & 0xffff
		communityNames = append(communityNames, fmt.Sprintf("%d:%d", front, back))
	}
	target.Communities = communityNames
}

func decodeLargeCommunities(target *RouteInfo, largeCommunities *[]*bgp.LargeCommunity) {
	var largeCommunitiesName []string
	if largeCommunities == nil {
		return
	}

	for _, community := range *largeCommunities {
		largeCommunitiesName = append(largeCommunitiesName, community.String())
	}
	target.LargeCommunities = largeCommunitiesName
}

func decodeValidation(target *RouteInfo, extendedCommunities *[]bgp.ExtendedCommunityInterface) {
	valid := bgp.VALIDATION_STATE_NOT_FOUND
	if extendedCommunities != nil {
		for _, ec := range *extendedCommunities {
			if val, ok := ec.(*bgp.ValidationExtended); ok {
				valid = val.State
				break
			}
		}
	}
	target.Validation = valid
}

func decodeRouteInfoMinimal(path *apiutil.Path) RouteInfo {
	var (
		nexthop             *netip.Addr
		mpReach             *bgp.PathAttributeMpReachNLRI
		asPath              *[]bgp.AsPathParamInterface
		multiExitDisc       *uint32
		localPref           *uint32
		extendedCommunities *[]bgp.ExtendedCommunityInterface
	)
	for _, a := range path.Attrs {
		switch a.GetType() {
		case bgp.BGP_ATTR_TYPE_NEXT_HOP:
			nexthop = &a.(*bgp.PathAttributeNextHop).Value
		case bgp.BGP_ATTR_TYPE_MP_REACH_NLRI:
			mpReach = a.(*bgp.PathAttributeMpReachNLRI)
		case bgp.BGP_ATTR_TYPE_AS_PATH:
			asPath = &a.(*bgp.PathAttributeAsPath).Value

		case bgp.BGP_ATTR_TYPE_MULTI_EXIT_DISC:
			multiExitDisc = &a.(*bgp.PathAttributeMultiExitDisc).Value
		case bgp.BGP_ATTR_TYPE_LOCAL_PREF:
			localPref = &a.(*bgp.PathAttributeLocalPref).Value
		case bgp.BGP_ATTR_TYPE_EXTENDED_COMMUNITIES:
			extendedCommunities = &a.(*bgp.PathAttributeExtendedCommunities).Value
		}
	}
	result := &RouteInfo{}
	decodeNextHop(result, nexthop, mpReach)
	decodeAsPath(result, asPath)
	decodeValidation(result, extendedCommunities)
	result.Best = path.Best

	if localPref != nil {
		result.LocalPref = *localPref
	}
	if multiExitDisc != nil {
		result.Med = *multiExitDisc
	}

	return *result
}

func decodeOrigin(target *RouteInfo, origin *uint8) {
	var originValue = OriginValue(255)
	if origin != nil {
		originValue = OriginValue(*origin)
	}
	target.Origin = originValue
}

func decodeNextHop(target *RouteInfo, nexthop *netip.Addr, mpReach *bgp.PathAttributeMpReachNLRI) {
	if nexthop != nil {
		target.NextHop = nexthop.String()
	} else if mpReach != nil {
		target.NextHop = mpReach.Nexthop.String()
	} else {
		target.NextHop = "N/A"
	}
}

func (r *Router) getCached(key cacheKey) ([]RouteInfo, bool) {
	if r.cache == nil {
		return nil, false
	}
	return r.cache.Get(key)
}

func (r *Router) setCached(key cacheKey, value []RouteInfo) {
	if r.cache == nil {
		return
	}
	r.cache.Add(key, value)
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
