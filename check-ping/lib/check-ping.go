package checkping

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/mackerelio/checkers"
	"github.com/tatsushid/go-fastping"
)

var opts struct {
	Warning  string `short:"w" long:"warning" value-name:"N, N%" description:"Exit with WARNING status if RTA less than N (ms) or N% of packet loss"`
	Critical string `short:"c" long:"critical" value-name:"N, N%" description:"Exit with CRITICAL status if less than N units or N% of disk are free"`
	Host     string `short:"H" long:"host" value-name:"Host" description:"Host name or IP Address to send ping"`
	Packets  int    `short:"p" long:"packets" value-name:"Packets" description:"Packet counts to send"`
	Timeout  int    `short:"t" long:"timeout" value-name:"Timeout" description:"Timeout (sec)"`
}

var pingTimeout time.Duration

// Do the plugin
func Do() {
	ckr := run(os.Args[1:])
	ckr.Name = "Ping"
	ckr.Exit()
}

func run(args []string) *checkers.Checker {
	_, err := flags.ParseArgs(&opts, args)
	if err != nil {
		os.Exit(1)
	}

	if opts.Host == "" {
		return checkers.Unknown(fmt.Sprintf("Host is required"))
	}

	// Default vaules
	setOptsDefaultString(&opts.Warning, "800, 20%")
	setOptsDefaultString(&opts.Critical, "1000, 40%")
	if opts.Packets == 0 {
		opts.Packets = 5
	}
	if opts.Timeout == 0 {
		opts.Timeout = 10
	}

	// Parse/Reset Thresholds
	warningRTT, warningPacketLoss, err := parseThresholds(opts.Warning)
	if err != nil {
		return checkers.Unknown(err.Error())
	}

	criticalRTT, criticalPacketLoss, err := parseThresholds(opts.Critical)
	if err != nil {
		return checkers.Unknown(err.Error())
	}

	if warningRTT > criticalRTT {
		warningRTT = criticalRTT
	}
	if warningPacketLoss > criticalPacketLoss {
		warningPacketLoss = criticalPacketLoss
	}

	// Check
	recvs := make([]time.Duration, 0)
	idlePkts := 0

	for range make([]struct{}, opts.Packets) {
		rtt, idle, err := ping()
		if idle || err != nil {
			idlePkts++
		} else {
			recvs = append(recvs, rtt)
		}
	}

	packetsSent := len(recvs) + idlePkts
	packetsReceived := len(recvs)

	totalRTT := time.Duration(0)
	for _, val := range recvs {
		totalRTT += val
	}

	var avgRTT time.Duration
	if len(recvs) == 0 {
		avgRTT = time.Duration(0)
	} else {
		avgRTT = totalRTT / time.Duration(len(recvs))
	}

	packetLoss := float64((1 - packetsReceived/packetsSent) * 100.0)

	msg := fmt.Sprintf(
		"Sent: %v, Recv: %v, RTT(Avg): %.3fms, PacketLoss %.0f%%",
		packetsSent,
		packetsReceived,
		float64(avgRTT)/float64(time.Millisecond),
		packetLoss)

	if !(packetsSent == opts.Packets) {
		return checkers.Unknown(msg)
	}

	if packetLoss < warningPacketLoss &&
		avgRTT < warningRTT {
		return checkers.Ok(msg)
	}

	if packetLoss >= criticalPacketLoss {
		return checkers.Critical(fmt.Sprint("Too many PacketLoss. ", msg))
	} else if avgRTT >= criticalRTT {
		return checkers.Critical(fmt.Sprint("Too long RTT. ", msg))
	} else if packetLoss >= warningPacketLoss {
		return checkers.Warning(fmt.Sprint("Too many PacketLoss. ", msg))
	} else if avgRTT >= warningRTT {
		return checkers.Warning(fmt.Sprint("Too long RTT. ", msg))
	}
	return checkers.Unknown("Unexpected reach to end of main")
}

func ping() (rtt time.Duration, idle bool, err error) {

	pinger := fastping.NewPinger()

	recvCh := make(chan time.Duration)
	idleCh := make(chan struct{})

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		for {
			select {
			case d := <-recvCh:
				rtt = d
				pinger.Stop()
				wg.Done()
			case <-idleCh:
				idle = true
				wg.Done()
			}
		}
	}()

	ra, err := net.ResolveIPAddr("ip4:icmp", opts.Host)
	if err != nil {
		return rtt, idle, err
	}
	// pingerのaddrsはkeyがaddr.String()なので同じアドレスは複数AddIPAddrできない
	// p.addrs[addr.String()] = &net.IPAddr{IP: addr}
	pinger.AddIPAddr(ra)

	pinger.MaxRTT = time.Duration(opts.Timeout) * time.Second

	pinger.OnRecv = func(addr *net.IPAddr, rtt time.Duration) {
		recvCh <- rtt

	}
	pinger.OnIdle = func() {
		idleCh <- struct{}{}
	}

	err = pinger.Run()
	if err != nil {
		return rtt, idle, err
	}
	wg.Wait()
	return rtt, idle, err
}

func setOptsDefaultString(v *string, val string) {
	if *v == "" {
		*v = val
	}
}

func parseThresholds(arg string) (rttThreshold time.Duration, packetLossThreshold float64, err error) {

	args := strings.Split(arg, ",")
	if len(args) != 2 {
		return 0, 0, fmt.Errorf("threshold %v is invalid format", arg)
	}
	args[0] = strings.Trim(args[0], " ")
	rttValue, err := strconv.Atoi(args[0])
	if err != nil {
		return 0, 0, fmt.Errorf("threshold %v is invalid. err=%v", arg, err.Error())
	}
	rttThreshold = time.Duration(rttValue) * time.Millisecond

	args[1] = strings.Trim(args[1], " ")
	args[1] = strings.Trim(args[1], "%")
	packetLossThreshold, err = strconv.ParseFloat(args[1], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("threshold %v is invalid. err=%v", arg, err.Error())
	}

	return rttThreshold, packetLossThreshold, err
}
