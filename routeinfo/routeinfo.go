package routeinfo

import (
	"context"
	"fmt"
	"net"
	"time"

	"log"

	api "github.com/osrg/gobgp/v3/api"
	gobgplog "github.com/osrg/gobgp/v3/pkg/log"
	"github.com/osrg/gobgp/v3/pkg/server"
)

type RouteInfoServer struct {
	Asn      uint32             `yaml:"asn"`
	RouterId string             `yaml:"routerid"`
	Routers  map[string]*Router `yaml:"routers"`
}

func (rs *RouteInfoServer) getBgpInstance() *server.BgpServer {
	server := server.NewBgpServer(server.LoggerOption(&silentLogger{}))
	go server.Serve()

	if rs.RouterId == "" {
		rs.RouterId = "255.255.255.255" // we should be able to get away with this
	}

	// global configuration
	if err := server.StartBgp(context.Background(), &api.StartBgpRequest{
		Global: &api.Global{
			Asn:        rs.Asn,
			RouterId:   rs.RouterId,
			ListenPort: -1, // gobgp won't listen on tcp:179
		},
	}); err != nil {
		log.Fatal(err)
	}

	server.WatchEvent(context.Background(), &api.WatchEventRequest{Peer: &api.WatchEventRequest_Peer{}}, func(r *api.WatchEventResponse) {
		if p := r.GetPeer(); p != nil {
			peer := p.GetPeer().State
			if peer.NeighborAddress == "<nil>" {
				// I don't know why this happens, but I don't want it logged.
				return
			}
			log.Printf("[info] Peer %d/%s is in state '%s'", peer.PeerAsn, peer.NeighborAddress, peer.SessionState)
		}
	})

	return server
}

func (rs *RouteInfoServer) Init() {
	for name, r := range rs.Routers {
		if len(r.Neighbors) == 0 {
			log.Fatalf("[error] unconfigured router %s\n", name)
		}
		if r.Asn == 0 {
			r.Asn = rs.Asn
		}
		if r.GobgpServer == nil {
			r.GobgpServer = rs.getBgpInstance()
		}
		r.Connect()
	}
}

type Router struct {
	Name        string   `yaml:"name"`
	Asn         uint32   `yaml:"asn"`
	Neighbors   []string `yaml:"neighbors"`
	GobgpServer *server.BgpServer
}

func (r *Router) Connect() {
	for _, addr := range r.Neighbors {
		var parsed net.IP
		if parsed = net.ParseIP(addr); parsed == nil {
			log.Printf("[error] Invalid address: %s", addr)
		}

		if err := r.GobgpServer.AddPeer(context.Background(), &api.AddPeerRequest{
			Peer: &api.Peer{
				Conf: &api.PeerConf{
					NeighborAddress: addr,
					PeerAsn:         r.Asn,
				},
			},
		}); err != nil {
			log.Fatal(err)
		}

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
	return r.lookup(address, api.TableLookupPrefix_SHORTER)
}

func (r *Router) LookupLonger(address string) []RouteInfo {
	if parsed := net.ParseIP(address); parsed != nil {
		if addr4 := parsed.To4(); addr4 != nil {
			address += "/32"
		} else {
			address += "/128"
		}
	}
	return r.lookup(address, api.TableLookupPrefix_LONGER)
}

func (r *Router) Lookup(address string) []RouteInfo {
	if parsed, _, _ := net.ParseCIDR(address); parsed != nil {
		address = parsed.String()
	}
	return r.lookup(address, api.TableLookupPrefix_EXACT)
}

func (r *Router) lookup(address string, lookupType api.TableLookupPrefix_Type) []RouteInfo {
	// determine AFI
	var parsed net.IP
	var afi api.Family_Afi
	if parsed = net.ParseIP(address); parsed != nil {
	} else if parsed, _, _ = net.ParseCIDR(address); parsed != nil {
	} else {
		log.Printf("[error] Invalid address: %s", address)
		return nil
	}
	if addr4 := parsed.To4(); addr4 != nil {
		afi = api.Family_AFI_IP
	} else {
		afi = api.Family_AFI_IP6
	}

	// build request
	family := &api.Family{Afi: afi, Safi: api.Family_SAFI_UNICAST}
	prefix := &api.TableLookupPrefix{Prefix: address, Type: lookupType}
	req := &api.ListPathRequest{TableType: api.TableType_GLOBAL, Family: family, Prefixes: []*api.TableLookupPrefix{prefix}}

	// get answer
	var destination *api.Destination
	r.GobgpServer.ListPath(context.Background(), req, func(d *api.Destination) {
		destination = d
	})

	// no answer here
	if destination == nil {
		log.Printf("[warning] No destination returned for %s.\n", address)
		return nil
	}

	// generate a result per path returned
	var results []RouteInfo
	for _, path := range destination.Paths {
		var result RouteInfo
		var Origin = &api.OriginAttribute{}
		var AsPath = &api.AsPathAttribute{}
		var MultiExitDisc = &api.MultiExitDiscAttribute{}
		var LocalPref = &api.LocalPrefAttribute{}
		var Communities = &api.CommunitiesAttribute{}
		var LargeCommunities = &api.LargeCommunitiesAttribute{}
		var ExtendedCommunities = &api.ExtendedCommunitiesAttribute{}
		var MpReachNLRI = &api.MpReachNLRIAttribute{}
		var NextHop = &api.NextHopAttribute{}
		var AtomicAggregate = &api.AtomicAggregateAttribute{}
		var Aggregator = &api.AggregatorAttribute{}
		var ClusterList = &api.ClusterListAttribute{}
		var OriginatorId = &api.OriginatorIdAttribute{}

		for _, pattr := range path.Pattrs {
			if err := pattr.UnmarshalTo(Origin); err == nil { // TODO: this is useless and won't be read
				continue
			} else if err := pattr.UnmarshalTo(AsPath); err == nil {
				continue
			} else if err := pattr.UnmarshalTo(MultiExitDisc); err == nil {
				continue
			} else if err := pattr.UnmarshalTo(LocalPref); err == nil {
				continue
			} else if err := pattr.UnmarshalTo(Communities); err == nil {
				continue
			} else if err := pattr.UnmarshalTo(LargeCommunities); err == nil {
				continue
			} else if err := pattr.UnmarshalTo(ExtendedCommunities); err == nil {
				continue
			} else if err := pattr.UnmarshalTo(MpReachNLRI); err == nil {
				continue
			} else if err := pattr.UnmarshalTo(NextHop); err == nil { // TODO: never actually seen this one
				continue
			} else if err := pattr.UnmarshalTo(AtomicAggregate); err == nil { // not used
				continue
			} else if err := pattr.UnmarshalTo(Aggregator); err == nil { //not used
				continue
			} else if err := pattr.UnmarshalTo(ClusterList); err == nil { //not used
				continue
			} else if err := pattr.UnmarshalTo(OriginatorId); err == nil { //not used
				continue
			} else {
				log.Printf("[warning] Path attribute decode not implemented for this object: %+v\n", pattr)
			}
		}

		var Prefix = &api.IPAddressPrefix{}
		if err := path.Nlri.UnmarshalTo(Prefix); err != nil {
			log.Printf("[warning] No prefix found for this destination path: %+v\n", path)
		}

		// decode nexthop
		var nexthop string
		if NextHop.NextHop != "" {
			nexthop = NextHop.NextHop
		} else if len(MpReachNLRI.NextHops) > 0 {
			nexthop = MpReachNLRI.NextHops[0]
		} else {
			nexthop = "N/A"
		}

		// decode aspath
		var aspath []uint32
		for _, segment := range AsPath.Segments {
			aspath = append(aspath, segment.Numbers...)
		}

		// decode communities
		var communities []string
		for _, community := range Communities.Communities {
			front := community >> 16
			back := community & 0xff
			communities = append(communities, fmt.Sprintf("%d:%d", front, back))
		}

		// decode large communities
		var largecommunities []string
		for _, community := range LargeCommunities.Communities {
			largecommunities = append(largecommunities, fmt.Sprintf("%d:%d:%d", community.GlobalAdmin, community.LocalData1, community.LocalData2))
		}

		// partly decode extended communities
		var valid = ValidationStatus(255)
		var ValidationExtended = &api.ValidationExtended{}
		for _, ec := range ExtendedCommunities.Communities {
			if err := ec.UnmarshalTo(ValidationExtended); err == nil {
				valid = ValidationStatus(ValidationExtended.State)
				break
			}
		}

		var origin = OriginValue(255)
		if Origin != nil {
			origin = OriginValue(Origin.Origin)
		}

		result = RouteInfo{
			AsPath:           aspath,
			Best:             path.Best,
			Communities:      communities,
			LargeCommunities: largecommunities,
			LocalPref:        LocalPref.LocalPref,
			Med:              MultiExitDisc.Med,
			NextHop:          nexthop,
			OriginAs:         aspath[len(aspath)-1],
			Origin:           origin,
			Peer:             path.NeighborIp,
			Prefix:           fmt.Sprintf("%s/%d", Prefix.Prefix, Prefix.PrefixLen),
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

func (r *Router) Status() (bool, bool) {
	var connected = true
	var ready = true
	for _, addr := range r.Neighbors {
		r.GobgpServer.ListPeer(context.Background(), &api.ListPeerRequest{Address: addr}, func(p *api.Peer) {
			connected = connected && p.State.SessionState == api.PeerState_ESTABLISHED
			for _, a := range p.AfiSafis {
				s := a.MpGracefulRestart.State
				ready = ready && s.EndOfRibReceived
			}
		})
	}
	return connected, ready
}

func (r *Router) WaitForEOR() {
	var ready bool
	for !ready {
		time.Sleep(time.Second * 1)
		_, ready = r.Status()
	}
}

type silentLogger struct {
}

func (l *silentLogger) Panic(msg string, fields gobgplog.Fields) {
}

func (l *silentLogger) Fatal(msg string, fields gobgplog.Fields) {
}

func (l *silentLogger) Error(msg string, fields gobgplog.Fields) {
}

func (l *silentLogger) Warn(msg string, fields gobgplog.Fields) {
}

func (l *silentLogger) Info(msg string, fields gobgplog.Fields) {
}

func (l *silentLogger) Debug(msg string, fields gobgplog.Fields) {
}

func (l *silentLogger) SetLevel(level gobgplog.LogLevel) {
}

func (l *silentLogger) GetLevel() gobgplog.LogLevel {
	return gobgplog.PanicLevel
}
