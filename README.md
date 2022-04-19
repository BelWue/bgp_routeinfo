# BelWü RouteInfo

This is a small package for extracting route information from BGP sessions.
This is done by directly establishing sessions with a number of peers using
[GoBGP](https://github.com/osrg/gobgp) and abstracting away the more complex
methods.


## Getting Started

1. Check out the provided `example_config.yml` and write your own version
2. Configure BGP sessions to your looking glass server. In most cases it
   makes sense to run the routeinfo-server as iBGP route-reflector client.
3. Run the API server or use RouteInfo as module in your own software.

You can run `cmd/routeinfo_server/main.go` to start a JSON/HTTP API server
and use the files in `lookingglass` to set up a web-frontend that can
query that API server. It is neccessary to edit `config.js`, but
`lookingglass.html` and `style.css` should also be seen as examples and can
be adapted or integrated into an existing website.

You can also use this in your own application. There's an example in
`cmd/example`, but it mainly consists of doing a YAML Unmarshal into an empty
RouteInfoServer object and using its `Lookup` methods.
