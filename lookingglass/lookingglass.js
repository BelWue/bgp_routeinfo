// TODO
// BropathEwsersupport-Check
// if ('content' in document.createElement('template')) {

var statusURL = lgSettings.routeinfoAPI.URL + "/status";
var prefixURL = lgSettings.routeinfoAPI.URL + "/prefix";

// RFC 6811
var validityMapping = {
    0: "Valid",
    1: "NotFound",
    2: "Invalid"
}

// RFC 4271
var originMapping = {
    0: "IGP",
    1: "EGP",
    2: "Incomplete"
}

function pathCompare(a, b) {
    // best path wins for readability
    if (!a.best && b.best) return -1;
    if (a.best && !b.best) return 1;

    // localpref (higher is better)
    if (a.localpref < b.localpref) return -1;
    if (a.localpref > b.localpref) return 1;

    // as-path length (shorter is better)
    if (a.aspath.length > b.aspath.length) return -1;
    if (a.aspath.length < b.aspath.length) return 1;

    // origin IGP > EGP > Incomplete
    if (a.origin > b.origin) return -1;
    if (a.origin < b.origin) return 1;

    // MED (lower is better)
    if (a.med > b.med) return -1;
    if (a.med < b.med) return 1;

    // TODO find a way to check if the neighbor larned the path via iBGP
    // eBGP wins over iBGP

    // more esoteric comparisons could happen here
    // ignored for looking-glass purposes
    return 0;
}

function addTextElement(tag, text, container, classes = []) {
    var element = document.createElement(tag);
    classes.forEach(function(classname){
        element.classList.add(classname);
    });
    var text = document.createTextNode(text);
    element.appendChild(text);
    container.appendChild(element);
}

function newTag(text) {
    var tagTemplate = document.querySelector("#lg-template-tag");
    var tag = document.importNode(tagTemplate.content, true);
    var tagElement = tag.querySelector("#lg-tag");
    tagElement.textContent = text;
    var tagInfo = lgSettings.tags[text];
    if (tagInfo) {
        tagElement.classList.add(`lg-tag-class-${tagInfo.class}`);
        tagElement.classList.add("lg-tag-tooltip");
        var tooltipText = document.createElement("span");
        tooltipText.textContent = tagInfo.description;
        tooltipText.classList.add("lg-tooltiptext")
        tagElement.appendChild(tooltipText);
    }
    return tag;
}

function addPathElement(path, container) {
    // path block
    var pathElementTemplate = document.querySelector("#lg-template-path");
    var pathElement = document.importNode(pathElementTemplate.content, true);

    // path data (as-path, next-hop, timestamp, metrics)
    if ("aspath" in path && path.aspath) {
        pathElement.querySelector("#lg-path-aspath").textContent = path.aspath.join(" ");
    } else {
        pathElement.querySelector("#lg-path-aspath").textContent = "(local)";
    }
    pathElement.querySelector("#lg-path-nexthop").textContent = `via ${path.nexthop} (since ${path.timestamp})`;
    pathElement.querySelector("#lg-path-metrics").textContent = `Localpref: ${path.localpref}, MED: ${path.med}, Origin: ${originMapping[path.origin]}`;

    // tags (bestpath, ROV-status)
    var tagSection = pathElement.querySelector("#lg-path-tags");
    if (path.best === true) {
        pathElement.querySelector("#lg-path").classList.add("lg-path-best");
        tagSection.appendChild(newTag("best"));
    }
    tagSection.appendChild(newTag(`RPKI ${validityMapping[path.validation]}`));

    // communities and large-communities
    var tagSection = pathElement.querySelector("#lg-path-tags");
    if (path.communities) {
        path.communities.forEach(function(community){
            tagSection.appendChild(newTag(community));
        });
    }
    if (path.largecommunities) {
        path.largecommunities.forEach(function(community){
            tagSection.appendChild(newTag(community));
        });
    }

    // append the path to the container
    container.appendChild(pathElement);
}

function newWarningMessage(text) {
    var oopsie = document.createElement("p");
    oopsie.textContent = text;
    oopsie.classList.add("lg-warning");
    return oopsie;
}

function displayStatus(data, container) {
    // TODO check errors
    if (!("results" in data) || (data.results == null) || (data.results.length == 0)) {
        container.appendChild(newWarningMessage("The route-server is not available at the moment. Please try again later."));
        return;
    }
    var routers = data.results;
    var routerCount = 0;
    routers.forEach(function(router){
        if (router.ready) {
            routerCount++;
        }
    });
    if (routerCount == 0) {
        container.appendChild(newWarningMessage("No routers are available for lookups at the moment. Please try again later."));
        return;
    }

    var querySection = document.querySelector("#lg-template-query");
    var clone = document.importNode(querySection.content, true);
    clone.querySelector("#lg-query-heading").textContent = "Routing Info is Available for " + routerCount + " Routers";
    var select = clone.querySelector("#lg-query-router");
    routers.forEach(function(router){
        let option = document.createElement("option");
        option.value = router.router;
        option.textContent = router.router;
        if (!router.ready) {
            option.disabled = true;
        }
        select.appendChild(option);
    });
    select.onchange = validateRouterSelect;

    container.appendChild(clone);

    // load query data from URL and (if a query is already there) run it
    loadQuery();
}

function addPrefixResult(result, container) {
    // TODO check errors
    var prefixRouterTemplate = document.querySelector("#lg-template-prefix-router");
    var clone = document.importNode(prefixRouterTemplate.content, true);
    var prefixRouterSection = clone.querySelector("#lg-prefix-router");
    clone.querySelector("#lg-prefix-router-name").textContent = result.router;
    clone.querySelector("#lg-prefix-router-prefix").textContent = result.prefix;
    result.paths.sort(pathCompare);
    result.paths.reverse();
    result.paths.forEach(function(path){
        addPathElement(path, prefixRouterSection);
    });
    container.appendChild(clone);
}

function prefixResultsToGraph(results) {
    var localName = lgSettings.graph.localASName;

    // build a list with all connections between any two AS
    var pathElements = [];
    results.forEach(function(router){
        router.paths.forEach(function(path){
            // empty paths are returned as null instead of empty array
            var pathlen = 0;
            if (path.aspath && path.aspath.length) {
                pathlen = path.aspath.length;
            }

            if (pathlen == 0) {
                // loop to self ("local")
                if (lgSettings.graph.drawLocalAsLoop) {
                    pathElements.push({
                        "best": path.best,
                        "from": localName,
                        "to": localName
                    });
                }
            } else {
                // arrow to first element
                pathElements.push({
                    "best": path.best,
                    "from": localName,
                    "to": path.aspath[0]
                });
            }

            // arrows for all following aspath elements
            for (let i = 0; i < (pathlen - 1); i++) {
                if (path.aspath[i] == path.aspath[i+1] && !lgSettings.graph.drawPrepends) {
                    // don't draw prepended paths with loops
                    continue;
                }
                pathElements.push({
                    "best": path.best,
                    "from": path.aspath[i],
                    "to": path.aspath[i+1]
                });
            }
        });
    });

    // deduplicate paths
    var deduplicatedPaths = [];
    for (const path of pathElements) {
        var found = false;
        for (const dedupedPath of deduplicatedPaths) {
            if ((path.from == dedupedPath.from) && (path.to == dedupedPath.to)) {
                // path is already in deduplicatedPaths
                // set best == true if one of them was marked best
                dedupedPath.best = dedupedPath.best || path.best;
                found = true;
                break;
            }
        }
        if (!found) {
            deduplicatedPaths.push(path);
        }
    }

    // build graph definition in text form
    graphDefinition = `flowchart LR\n  ${localName}`;
    deduplicatedPaths.forEach(function(path){
        // dashed arrow by default, normal arrow for best path
        var arrow = "-.->";
        if (path.best) {
            arrow = "-->";
        }
        graphDefinition += `\n  ${path.from} ${arrow} ${path.to}`;
    });
    return graphDefinition;
}

function displayPrefix(data, container) {
    // activate loading queries on hash change (gets disabled before on new queries)
    window.onhashchange = loadQuery;

    // get a new result block
    var resultPrefixSection = document.querySelector("#lg-template-result-prefix");
    var clone = document.importNode(resultPrefixSection.content, true);
    var resultBlock = clone.querySelector("#lg-result-block");

    if (!("results" in data) || (data.results == null) || (data.results.length == 0)) {
        resultBlock.appendChild(newWarningMessage("No prefixes found."));
        container.appendChild(clone);
        return;
    }

    // fill the block with data
    var prefixResults = resultBlock.querySelector("#lg-prefix-results");
    data.results.forEach(function(result){
        addPrefixResult(result, prefixResults);
    });

    // append the block to the container
    container.appendChild(clone);

    // BGP graph
    if (lgSettings.graph.enabled) {
        var bgpGraphTemplate = document.querySelector("#lg-template-bgp-graph")
        var bgpGraphBlock = document.importNode(bgpGraphTemplate.content, true);
        var bgpGraph = bgpGraphBlock.querySelector("#lg-bgp-graph");
        bgpGraph.textContent = prefixResultsToGraph(data.results);
        resultBlock.appendChild(bgpGraphBlock);
        mermaid.init();
    }
}

function queryStatus() {
    var container = document.querySelector("#lg-container");
    fetch(statusURL, {
        "method": "GET",
        "cache": "no-store"
    })
        .then(response => response.json())
        .then(response => displayStatus(response, container))
    // TODO handle .catch
}

function validateRouterSelect() {
    // check if the router placeholder is selected
    var form = document.querySelector("#lg-query-form");
    var formSelectRouter = form.querySelector("#lg-query-router")
    if (formSelectRouter.value == "-") {
        formSelectRouter.setCustomValidity("Please select a router.");
        return;
    } else {
        formSelectRouter.setCustomValidity("");
    }
}

function queryPrefix() {
    // check the form validity
    validateRouterSelect();
    var form = document.querySelector("#lg-query-form");
    if (!form.reportValidity()) {
        return;
    }

    // disable request on hash change (will be enabled in displayPrefix when the response has arrived)
    window.onhashchange = function(){ return; }

    // clear the last result
    var lastResultSection = document.querySelector("#lg-result-block");
    if (lastResultSection) { lastResultSection.remove(); }

    var container = document.querySelector("#lg-container");

    var prefix = document.querySelector("#lg-query-prefix").value;
    var routerSelect = document.querySelector("#lg-query-router");
    var router = routerSelect.options[routerSelect.selectedIndex].value;

    // store query data in the URL hash part
    var formData = new FormData();
    formData.append("prefix", prefix);
    formData.append("router", router);
    var hash = new URLSearchParams(formData).toString();

    window.location.hash = hash;

    // hack: URL() doesn't work with relative URLs...
    var url = new URL((new Request(prefixURL)).url);
    url.searchParams.append("prefix", prefix);
    url.searchParams.append("router", router);

    // TODO loading screen

    fetch(url, {
        "method": "GET",
        "cache": "no-store"
    })
        .then(response => response.json())
        .then(response => displayPrefix(response, container))
    // TODO handle .catch

    // display a link to raw JSON output
    if (lgSettings.routeinfoAPI.showAPILink) {
        // check if an old link exists and remove it
        var lastAPILink = document.querySelector("#lg-query-block").querySelector("#lg-api-link-block");
        if (lastAPILink) {
            lastAPILink.href = url;
        } else {
            // copy the template and set the right URL
            var apiLinkTemplate = document.querySelector("#lg-template-api-link");
            var apiLink = document.importNode(apiLinkTemplate.content, true);
            apiLink.querySelector("#lg-api-link").href = url;
            var querySection = document.querySelector("#lg-query-block");
            querySection.appendChild(apiLink);
        }
    }
}

function loadQuery() {
    // disable request on hash change (will be enabled in displayPrefix when the response has arrived)
    window.onhashchange = function(){ return; }

    // load the query from the anchor-part of the URL, if there is one
    var hash = window.location.hash.substring(1);
    if (!hash) { return; }
    var queryData = new URLSearchParams(hash);
    var prefix = queryData.get("prefix");
    var router = queryData.get("router");

    var prefixInputElement = document.querySelector("#lg-query-prefix");
    prefixInputElement.value = prefix;
    var routerSelectElement = document.querySelector("#lg-query-router");
    routerSelectElement.value = router;
    if (routerSelectElement.selectedIndex == -1) {
        routerSelectElement.value = "";
    }

    queryPrefix();
    window.onhashchange = loadQuery;
}

queryStatus();
