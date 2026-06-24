package trafficstats

type Stat struct {
	Engine    string
	ScopeType string
	ScopeKey  string
	Uplink    int64
	Downlink  int64
}
