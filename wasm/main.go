package main

import (
	// "bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"syscall/js"
	"time"

	// "golang.org/x/crypto/ssh"
	"tailscale.com/control/controlclient"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnlocal"
	"tailscale.com/ipn/ipnserver"
	"tailscale.com/ipn/store/mem"
	"tailscale.com/logpolicy"
	"tailscale.com/logtail"
	"tailscale.com/net/netcheck"
	"tailscale.com/net/netns"
	"tailscale.com/net/tsdial"
	"tailscale.com/safesocket"
	"tailscale.com/tailcfg"
	"tailscale.com/tsd"
	"tailscale.com/types/views"
	"tailscale.com/wgengine"
	"tailscale.com/wgengine/netstack"
	"tailscale.com/words"
)

// ControlURL defines the URL to be used for connection to Control.
var ControlURL = ipn.DefaultControlURL

func main() {
	js.Global().Set("newIPN", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) != 1 {
			log.Fatal("Usage: newIPN(config)")
			return nil
		}
		return newIPN(args[0])
	}))
	// Keep Go runtime alive, otherwise it will be shut down before newIPN gets
	// called.
	<-make(chan bool)
}

type jsIPN struct {
	dialer     *tsdial.Dialer
	srv        *ipnserver.Server
	lb         *ipnlocal.LocalBackend
	controlURL string
	authKey    string
	hostname   string
	netChecker *netcheck.Client
}

var jsIPNState = map[ipn.State]string{
	ipn.NoState:          "NoState",
	ipn.InUseOtherUser:   "InUseOtherUser",
	ipn.NeedsLogin:       "NeedsLogin",
	ipn.NeedsMachineAuth: "NeedsMachineAuth",
	ipn.Stopped:          "Stopped",
	ipn.Starting:         "Starting",
	ipn.Running:          "Running",
}

var jsMachineStatus = map[tailcfg.MachineStatus]string{
	tailcfg.MachineUnknown:      "MachineUnknown",
	tailcfg.MachineUnauthorized: "MachineUnauthorized",
	tailcfg.MachineAuthorized:   "MachineAuthorized",
	tailcfg.MachineInvalid:      "MachineInvalid",
}

func newIPN(jsConfig js.Value) map[string]any {
	netns.SetEnabled(false)

	var store ipn.StateStore
	if jsStateStorage := jsConfig.Get("stateStorage"); !jsStateStorage.IsUndefined() {
		store = &jsStateStore{jsStateStorage}
		js.Global().Get("console").Call("log", "Using state storage from JS")
	} else {
		store = new(mem.Store)
		js.Global().Get("console").Call("log", "Using state storage from memory")
	}

	controlURL := ControlURL
	if jsControlURL := jsConfig.Get("controlURL"); jsControlURL.Type() == js.TypeString {
		controlURL = jsControlURL.String()
	}

	var authKey string
	if jsAuthKey := jsConfig.Get("authKey"); jsAuthKey.Type() == js.TypeString {
		authKey = jsAuthKey.String()
	}

	var hostname string
	if jsHostname := jsConfig.Get("hostname"); jsHostname.Type() == js.TypeString {
		hostname = jsHostname.String()
	} else {
		hostname = generateHostname()
	}

	lpc := getOrCreateLogPolicyConfig(store)
	c := logtail.Config{
		Collection: lpc.Collection,
		PrivateID:  lpc.PrivateID,

		// Compressed requests set HTTP headers that are not supported by the
		// no-cors fetching mode:
		CompressLogs: false,

		HTTPC: &http.Client{Transport: &noCORSTransport{http.DefaultTransport}},
	}
	logtail := logtail.NewLogger(c, log.Printf)
	logf := logtail.Logf

	sys := new(tsd.System)
	sys.Set(store)
	dialer := &tsdial.Dialer{Logf: logf}
	eng, err := wgengine.NewUserspaceEngine(logf, wgengine.Config{
		Dialer:        dialer,
		SetSubsystem:  sys.Set,
		ControlKnobs:  sys.ControlKnobs(),
		HealthTracker: sys.HealthTracker(),
		Metrics:       sys.UserMetricsRegistry(),
	})
	if err != nil {
		log.Fatal(err)
	}
	sys.Set(eng)

	ns, err := netstack.Create(logf, sys.Tun.Get(), eng, sys.MagicSock.Get(), dialer, sys.DNSManager.Get(), sys.ProxyMapper(), nil)
	if err != nil {
		log.Fatalf("netstack.Create: %v", err)
	}
	sys.Set(ns)
	ns.ProcessLocalIPs = true
	ns.ProcessSubnets = true

	dialer.UseNetstackForIP = func(ip netip.Addr) bool {
		return true
	}
	dialer.NetstackDialTCP = func(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
		return ns.DialContextTCP(ctx, dst)
	}
	dialer.NetstackDialUDP = func(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
		return ns.DialContextUDP(ctx, dst)
	}
	sys.NetstackRouter.Set(true)
	sys.Tun.Get().Start()

	logid := lpc.PublicID
	srv := ipnserver.New(logf, logid, sys.NetMon.Get())
	lb, err := ipnlocal.NewLocalBackend(logf, logid, sys, controlclient.LoginEphemeral)
	if err != nil {
		log.Fatalf("ipnlocal.NewLocalBackend: %v", err)
	}
	if err := ns.Start(lb); err != nil {
		log.Fatalf("failed to start netstack: %v", err)
	}
	srv.SetLocalBackend(lb)

	// 初始化 netChecker
	netChecker := &netcheck.Client{
		NetMon:      sys.NetMon.Get(),
		Logf:        logf,
		Verbose:     true,
		UseDNSCache: true,
	}

	jsIPN := &jsIPN{
		dialer:     dialer,
		srv:        srv,
		lb:         lb,
		controlURL: controlURL,
		authKey:    authKey,
		hostname:   hostname,
		netChecker: netChecker,
	}

	return map[string]any{
		"run": js.FuncOf(func(this js.Value, args []js.Value) any {
			if len(args) != 1 {
				log.Fatal(`Usage: run({
					notifyState(state: int): void,
					notifyNetMap(netMap: object): void,
					notifyBrowseToURL(url: string): void,
					notifyPanicRecover(err: string): void,
				})`)
				return nil
			}
			jsIPN.run(args[0])
			return nil
		}),
		"login": js.FuncOf(func(this js.Value, args []js.Value) any {
			if len(args) != 0 {
				log.Printf("Usage: login()")
				return nil
			}
			jsIPN.login()
			return nil
		}),
		"netCheck": js.FuncOf(func(this js.Value, args []js.Value) any {
			// 创建 Promise
			handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				resolve := args[0]
				reject := args[1]

				go func() {
					// 检查 IPN 状态
					if jsIPN.lb.State() == ipn.NoState {
						reject.Invoke("Backend not initialized")
						return
					}

					// 获取 DERPMap
					derpMap := jsIPN.lb.DERPMap()
					// derpMapJSON, _ := json.Marshal(derpMap)
					// fmt.Printf("DERPMap: %s\n", string(derpMapJSON))
					if derpMap == nil {
						reject.Invoke("DERPMap not available")
						return
					}

					// 执行 netcheck
					report, err := jsIPN.netChecker.GetReport(context.Background(), derpMap, nil)
					if err != nil {
						reject.Invoke(err.Error())
						return
					}
					// 转换延迟数据
					formattedLatency := make(map[string]interface{})
					for regionID, latencyNanos := range report.RegionLatency {
						if region, ok := derpMap.Regions[regionID]; ok {
							// 构建详细的区域信息
							regionInfo := map[string]interface{}{
								"name":      region.RegionName,
								"code":      region.RegionCode,
								"location":  fmt.Sprintf("%.2f, %.2f", region.Latitude, region.Longitude),
								"latencyMS": float64(latencyNanos) / float64(time.Millisecond),
							}
							formattedLatency[fmt.Sprintf("%s (%s)", region.RegionName, region.RegionCode)] = regionInfo
						}
					}
					// 获取首选 DERP 的区域信息
					var preferredDERPInfo string
					if region, ok := derpMap.Regions[report.PreferredDERP]; ok {
						preferredDERPInfo = fmt.Sprintf("%s (%s)", region.RegionName, region.RegionCode)
					}

					// 构建返回数据
					result := map[string]interface{}{
						"preferredDERP": preferredDERPInfo,
						"regionLatency": formattedLatency,
						"udpSupported":  report.UDP,
						"globalV4":      report.GlobalV4.String(),
						"globalV6":      report.GlobalV6.String(),
					}

					// 转换为 JSON
					if jsonResult, err := json.Marshal(result); err == nil {
						resolve.Invoke(string(jsonResult))
					} else {
						reject.Invoke("Failed to marshal result")
					}
				}()
				return nil
			})

			// 返回新的 Promise
			promiseConstructor := js.Global().Get("Promise")
			return promiseConstructor.New(handler)
		}),
	}
}

func (i *jsIPN) run(jsCallbacks js.Value) {
	notifyState := func(state ipn.State) {
		jsCallbacks.Call("notifyState", jsIPNState[state])
	}
	notifyState(ipn.NoState)

	i.lb.SetNotifyCallback(func(n ipn.Notify) {
		// Panics in the notify callback are likely due to be due to bugs in
		// this bridging module (as opposed to actual bugs in Tailscale) and
		// thus may be recoverable. Let the UI know, and allow the user to
		// choose if they want to reload the page.
		defer func() {
			if r := recover(); r != nil {
				fmt.Println("Panic recovered:", r)
				jsCallbacks.Call("notifyPanicRecover", fmt.Sprint(r))
			}
		}()
		log.Printf("NOTIFY: %+v", n)
		if n.State != nil {
			notifyState(*n.State)
		}
		if nm := n.NetMap; nm != nil {
			jsNetMap := jsNetMap{
				Self: jsNetMapSelfNode{
					jsNetMapNode: jsNetMapNode{
						Name:       nm.Name,
						Addresses:  mapSliceView(nm.GetAddresses(), func(a netip.Prefix) string { return a.Addr().String() }),
						NodeKey:    nm.NodeKey.String(),
						MachineKey: nm.MachineKey.String(),
					},
					MachineStatus: jsMachineStatus[nm.GetMachineStatus()],
				},
				Peers: mapSlice(nm.Peers, func(p tailcfg.NodeView) jsNetMapPeerNode {
					name := p.Name()
					if name == "" {
						// In practice this should only happen for Hello.
						name = p.Hostinfo().Hostname()
					}
					addrs := make([]string, p.Addresses().Len())
					for i := range p.Addresses().Len() {
						addrs[i] = p.Addresses().At(i).Addr().String()
					}
					return jsNetMapPeerNode{
						jsNetMapNode: jsNetMapNode{
							Name:       name,
							Addresses:  addrs,
							MachineKey: p.Machine().String(),
							NodeKey:    p.Key().String(),
						},
						Online:              p.Online(),
						TailscaleSSHEnabled: p.Hostinfo().TailscaleSSHEnabled(),
					}
				}),
				LockedOut: nm.TKAEnabled && nm.SelfNode.KeySignature().Len() == 0,
			}
			if jsonNetMap, err := json.Marshal(jsNetMap); err == nil {
				jsCallbacks.Call("notifyNetMap", string(jsonNetMap))
			} else {
				log.Printf("Could not generate JSON netmap: %v", err)
			}
		}
		if n.BrowseToURL != nil {
			jsCallbacks.Call("notifyBrowseToURL", *n.BrowseToURL)
		}
	})

	// local backend
	go func() {
		err := i.lb.Start(ipn.Options{
			UpdatePrefs: &ipn.Prefs{
				ControlURL:  i.controlURL,
				RouteAll:    false,
				WantRunning: true,
				Hostname:    i.hostname,
			},
			AuthKey: i.authKey,
		})
		if err != nil {
			log.Printf("Start error: %v", err)
		}
	}()

	// ipn server
	go func() {
		ln, err := safesocket.Listen("")
		if err != nil {
			log.Fatalf("safesocket.Listen: %v", err)
		}

		err = i.srv.Run(context.Background(), ln)
		log.Fatalf("ipnserver.Run exited: %v", err)
	}()
}

func (i *jsIPN) login() {
	go i.lb.StartLoginInteractive(context.Background())
}

func (i *jsIPN) logout() {
	if i.lb.State() == ipn.NoState {
		log.Printf("Backend not running")
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		i.lb.Logout(ctx)
	}()
}

type jsNetMap struct {
	Self      jsNetMapSelfNode   `json:"self"`
	Peers     []jsNetMapPeerNode `json:"peers"`
	LockedOut bool               `json:"lockedOut"`
}

type jsNetMapNode struct {
	Name       string   `json:"name"`
	Addresses  []string `json:"addresses"`
	MachineKey string   `json:"machineKey"`
	NodeKey    string   `json:"nodeKey"`
}

type jsNetMapSelfNode struct {
	jsNetMapNode
	MachineStatus string `json:"machineStatus"`
}

type jsNetMapPeerNode struct {
	jsNetMapNode
	Online              *bool `json:"online,omitempty"`
	TailscaleSSHEnabled bool  `json:"tailscaleSSHEnabled"`
}

type jsStateStore struct {
	jsStateStorage js.Value
}

func (s *jsStateStore) ReadState(id ipn.StateKey) ([]byte, error) {
	jsValue := s.jsStateStorage.Call("getState", string(id))
	if jsValue.String() == "" {
		return nil, ipn.ErrStateNotExist
	}
	return hex.DecodeString(jsValue.String())
}

func (s *jsStateStore) WriteState(id ipn.StateKey, bs []byte) error {
	s.jsStateStorage.Call("setState", string(id), hex.EncodeToString(bs))
	return nil
}

func mapSlice[T any, M any](a []T, f func(T) M) []M {
	n := make([]M, len(a))
	for i, e := range a {
		n[i] = f(e)
	}
	return n
}

func mapSliceView[T any, M any](a views.Slice[T], f func(T) M) []M {
	n := make([]M, a.Len())
	for i := range a.Len() {
		n[i] = f(a.At(i))
	}
	return n
}

func filterSlice[T any](a []T, f func(T) bool) []T {
	n := make([]T, 0, len(a))
	for _, e := range a {
		if f(e) {
			n = append(n, e)
		}
	}
	return n
}

func generateHostname() string {
	tails := words.Tails()
	scales := words.Scales()
	if rand.IntN(2) == 0 {
		// JavaScript
		tails = filterSlice(tails, func(s string) bool { return strings.HasPrefix(s, "j") })
		scales = filterSlice(scales, func(s string) bool { return strings.HasPrefix(s, "s") })
	} else {
		// WebAssembly
		tails = filterSlice(tails, func(s string) bool { return strings.HasPrefix(s, "w") })
		scales = filterSlice(scales, func(s string) bool { return strings.HasPrefix(s, "a") })
	}

	tail := tails[rand.IntN(len(tails))]
	scale := scales[rand.IntN(len(scales))]
	return fmt.Sprintf("%s-%s", tail, scale)
}

const logPolicyStateKey = "log-policy"

func getOrCreateLogPolicyConfig(state ipn.StateStore) *logpolicy.Config {
	if configBytes, err := state.ReadState(logPolicyStateKey); err == nil {
		if config, err := logpolicy.ConfigFromBytes(configBytes); err == nil {
			js.Global().Get("console").Call("warn", "logpolicy.ConfigFromBytes")
			return config
		} else {
			log.Printf("Could not parse log policy config: %v", err)
			js.Global().Get("console").Call("warn", "Could not parse log policy config")
		}
	} else if err != ipn.ErrStateNotExist {
		log.Printf("Could not get log policy config from state store: %v", err)
		js.Global().Get("console").Call("warn", "Could not get log policy config from state store")
	}
	config := logpolicy.NewConfig(logtail.CollectionNode)
	if err := state.WriteState(logPolicyStateKey, config.ToBytes()); err != nil {
		log.Printf("Could not save log policy config to state store: %v", err)
		js.Global().Get("console").Call("warn", "Could not save log policy config to state store")
	}
	return config
}

/*
type Config struct {
    Collection     string          // collection name, a domain name
    PrivateID      logid.PrivateID // private ID for the primary log stream
    CopyPrivateID  logid.PrivateID // private ID for a log stream that is a superset of this log stream
    BaseURL        string          // if empty defaults to "https://log.tailscale.io"
    HTTPC          *http.Client    // if empty defaults to http.DefaultClient
    SkipClientTime bool            // if true, client_time is not written to logs
    LowMemory      bool            // if true, logtail minimizes memory use
    Clock          tstime.Clock    // if set, Clock.Now substitutes uses of time.Now
    Stderr         io.Writer       // if set, logs are sent here instead of os.Stderr
    StderrLevel    int             // max verbosity level to write to stderr; 0 means the non-verbose messages only
    Buffer         Buffer          // temp storage, if nil a MemoryBuffer
    CompressLogs   bool            // whether to compress the log uploads

    // MetricsDelta, if non-nil, is a func that returns an encoding
    // delta in clientmetrics to upload alongside existing logs.
    // It can return either an empty string (for nothing) or a string
    // that's safe to embed in a JSON string literal without further escaping.
    MetricsDelta func() string

    // FlushDelayFn, if non-nil is a func that returns how long to wait to
    // accumulate logs before uploading them. 0 or negative means to upload
    // immediately.
    //
    // If nil, a default value is used. (currently 2 seconds)
    FlushDelayFn func() time.Duration

    // IncludeProcID, if true, results in an ephemeral process identifier being
    // included in logs. The ID is random and not guaranteed to be globally
    // unique, but it can be used to distinguish between different instances
    // running with same PrivateID.
    IncludeProcID bool

    // IncludeProcSequence, if true, results in an ephemeral sequence number
    // being included in the logs. The sequence number is incremented for each
    // log message sent, but is not persisted across process restarts.
    IncludeProcSequence bool
}
*/

// noCORSTransport wraps a RoundTripper and forces the no-cors mode on requests,
// so that we can use it with non-CORS-aware servers.
type noCORSTransport struct {
	http.RoundTripper
}

func (t *noCORSTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("js.fetch:mode", "no-cors")
	resp, err := t.RoundTripper.RoundTrip(req)
	if err == nil {
		// In no-cors mode no response properties are returned. Populate just
		// the status so that callers do not think this was an error.
		resp.StatusCode = http.StatusOK
		resp.Status = http.StatusText(http.StatusOK)
	}
	return resp, err
}
