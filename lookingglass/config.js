lgSettings = {
    "routeinfoAPI": {
        "URL": "https://example.org:3000",  // URL of your API server (without trailing slash)
        "showAPILink": true,  // show "link to raw JSON result"
    },
    "graph": {
        "enabled": true,  // render a BGP tree (requires mermaid.js)
        "localASName": "My Network"  // display name of your AS
    },
    "tags": {
        // additional data for tags
        // "description" is displayed as tooltip
        // "class" is added as CSS class "lg-tag-class-$class" (i.e. "class": "best" will turn into ".lg-tag-class-best")
        // (can be used for styling, e.g. to turn blackhole-communities red)
        "best": {
            "description": "router-local best-path",
            "class": "best"
        },
        "RPKI NotFound": {
            "description": "RPKI origin validation status \"NotFound\"",
            "class": "rpki-notfound"
        },
        "RPKI Valid": {
            "description": "RPKI origin validation status \"Valid\"",
            "class": "rpki-valid"
        },
        "RPKI Invalid": {
            "description": "RPKI origin validation status \"Invalid\"",
            "class": "rpki-invalid"
        },
        "65535:0": {
            "description": "GRACEFUL_SHUTDOWN",
            "class": "action-restrict"
        },
        "65535:666": {
            "description": "BLACKHOLE",
            "class": "action-blackhole"
        },
        "65535:65281": {
            "description": "NO_EXPORT",
            "class": "action-restrict"
        },
        "65535:65282": {
            "description": "NO_ADVERTISE",
            "class": "action-restrict"
        },
        "65535:65283": {
            "description": "NO_EXPORT_SUBCONFED",
            "class": "action-restrict"
        },
        "65535:65284": {
            "description": "NOPEER",
            "class": "action-restrict"
        }
    }
}
