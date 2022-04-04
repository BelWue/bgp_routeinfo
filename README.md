# BelWü RouteInfo

This is a small package for extracting route information from BGP sessions.
This is done by directly establishing sessions with a number of peers using
[GoBGP](https://github.com/osrg/gobgp) and abstracting away the more complex
methods.

## Getting Started

1. Check out the provided `example_config.yml` and write your own version
2. Configure you routers to talk to you, on Cisco IOS-XR, it should look right about this:

```
conf
router bgp 553
 neighbor x.x.x.x
  remote-as 553
  description BGP RouteInfo
  update-source Loopback0
  address-family ipv4 unicast
   route-policy bgp-routeinfo-in in
   route-reflector-client
   route-policy bgp-routeinfo-out out
   soft-reconfiguration inbound always
  !
 !
!
 neighbor xy::z
  remote-as 553
  description BGP RouteInfo
  update-source Loopback0
  address-family ipv6 unicast
   route-policy bgp-routeinfo-in in
   route-reflector-client
   route-policy bgp-routeinfo-out out
   soft-reconfiguration inbound always
  !
 !
!
commit
end
```

Then use the API in your application. There's an example in `cmd/example`, but
it mainly consists of doing a YAML Unmarshal into an empty RouteInfoServer
object and using its `Lookup` methods.
