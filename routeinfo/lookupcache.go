package routeinfo

import (
	"time"

	"github.com/osrg/gobgp/v4/pkg/apiutil"
)

type cacheKey struct {
	Address    string
	LookupType apiutil.LookupOption
}

type cacheEntry struct {
	ExpiresAt time.Time
	Value     []RouteInfo
}
