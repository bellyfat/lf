package lf

import (
	"net"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NYTimes/gziphandler"
)

// Node is an instance of LF
type Node struct {
	udpSocket          *net.UDPConn
	httpServer         *http.Server
	backgroundThreadWG sync.WaitGroup

	db DB

	hosts       []*Host
	hostsByAddr map[packedAddress]*Host
	hostsLock   sync.RWMutex

	startTime uint64
	shutdown  uintptr
}

// NewNode creates and starts a node.
func NewNode(path string, port int) (*Node, error) {
	var err error
	n := new(Node)

	var laddr net.UDPAddr
	laddr.Port = int(port)
	n.udpSocket, err = net.ListenUDP("udp", &laddr)
	if err != nil {
		return nil, err
	}

	var ta net.TCPAddr
	ta.Port = int(port)
	httpTCPListener, err := net.ListenTCP("tcp", &ta)
	if err != nil {
		return nil, err
	}

	err = n.db.Open(path)
	if err != nil {
		return nil, err
	}

	n.hosts = make([]*Host, 0, 1024)
	n.hostsByAddr = make(map[packedAddress]*Host)
	n.startTime = TimeMs()

	// UDP receiver threads
	n.backgroundThreadWG.Add(runtime.NumCPU())
	for tc := 0; tc < runtime.NumCPU(); tc++ {
		go func() {
			var buf [16384]byte
			for atomic.LoadUintptr(&n.shutdown) == 0 {
				bytes, addr, err := n.udpSocket.ReadFromUDP(buf[:])
				if bytes > 0 && err == nil {
					n.GetHost(addr.IP, addr.Port, addr.Zone, true).handleIncomingPacket(n, buf[0:bytes])
				}
			}
			n.backgroundThreadWG.Done()
		}()
	}

	// HTTP server thread
	n.httpServer = &http.Server{
		MaxHeaderBytes: 4096,
		Handler:        gziphandler.GzipHandler(apiCreateHTTPServeMux(n)),
		IdleTimeout:    10 * time.Second,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   60 * time.Second}
	n.httpServer.SetKeepAlivesEnabled(true)
	n.backgroundThreadWG.Add(1)
	go func() {
		n.httpServer.Serve(httpTCPListener)
		n.httpServer.Close()
		n.backgroundThreadWG.Done()
	}()

	// Peer connection cleanup and ping thread
	n.backgroundThreadWG.Add(1)
	go func() {
		for atomic.LoadUintptr(&n.shutdown) == 0 {
			time.Sleep(time.Second)
			n.hostsLock.Lock()
			hostCount := 0
			now := TimeMs()
			for i := 0; i < len(n.hosts); i++ {
				if (now - n.hosts[i].LastReceive) > ProtoHostTimeout {
					delete(n.hostsByAddr, n.hosts[i].packedAddress)
				} else {
					if (now - n.hosts[i].LastSend) > (ProtoHostTimeout / 3) {
						n.hosts[i].Ping(n, false)
					}
					n.hosts[hostCount] = n.hosts[i]
					hostCount++
				}
			}
			n.hosts = n.hosts[0:hostCount]
			n.hostsLock.Unlock()
		}
		n.backgroundThreadWG.Done()
	}()

	return n, nil
}

// Stop terminates the running node. No methods should be called after this.
func (n *Node) Stop() {
	atomic.StoreUintptr(&n.shutdown, 1)
	n.udpSocket.Close()
	n.httpServer.Close()
	n.backgroundThreadWG.Wait()

	n.db.Close()

	WharrgarblFreeGlobalMemory()
}

// GetHost gets the Host object for a given address.
// If createIfMissing is true a new object is initialized if there is not one currently. Otherwise nil
// is returned if no host is known.
func (n *Node) GetHost(ip net.IP, port int, zone string, createIfMissing bool) *Host {
	var mapKey packedAddress
	mapKey.set(ip, port, zone)
	n.hostsLock.RLock()
	h := n.hostsByAddr[mapKey]
	n.hostsLock.RUnlock()
	if h == nil {
		if createIfMissing {
			h = &Host{
				packedAddress: mapKey,
				FirstReceive:  TimeMs(),
				RemoteAddress: net.UDPAddr{IP: ip, Port: port, Zone: zone},
				Latency:       -1}
			n.hostsLock.Lock()
			n.hosts = append(n.hosts, h)
			n.hostsByAddr[mapKey] = h
			n.hostsLock.Unlock()
		} else {
			return nil
		}
	}
	return h
}

// Try makes an attempt to contact a peer if it's not already connected to us.
func (n *Node) Try(ip []byte, port int, zone string) {
	h := n.GetHost(ip, port, zone, true)
	if !h.Connected() {
		h.Ping(n, false)
	}
}

// AddRecord attempts to add a record to this node's database.
func (n *Node) AddRecord(recordData []byte) error {
	return nil
}
