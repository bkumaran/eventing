package consumer

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/couchbase/eventing/common"
	"github.com/couchbase/eventing/logging"
	"github.com/couchbase/eventing/util"
	"github.com/couchbase/gocb"
)

func newDebugClient(c *Consumer, appName, eventingPort, tcpPort, workerName string) *debugClient {
	return &debugClient{
		appName:        appName,
		consumerHandle: c,
		debugTCPPort:   tcpPort,
		eventingPort:   eventingPort,
		workerName:     workerName,
	}
}

func (c *debugClient) Serve() {
	c.cmd = exec.Command("eventing-consumer", c.appName, c.debugTCPPort, c.workerName,
		strconv.Itoa(c.consumerHandle.socketWriteBatchSize), c.eventingPort, "debug")

	err := c.cmd.Start()
	if err != nil {
		logging.Errorf("CRDCL[%s:%s:%s:%d] Failed to spawn c++ worker for debugger",
			c.appName, c.workerName, c.debugTCPPort, c.osPid)
	} else {
		logging.Infof("CRDCL[%s:%s:%s:%d] C++ worker launched for debugger",
			c.appName, c.workerName, c.debugTCPPort, c.osPid)
	}

	c.cmd.Wait()

	logging.Debugf("CRDCL[%s:%s:%s:%d] Exiting C++ worker spawned for debugger",
		c.appName, c.workerName, c.debugTCPPort, c.osPid)
}

func (c *debugClient) Stop() {
	logging.Debugf("CRDCL[%s:%s:%s:%d] Stopping C++ worker spawned for debugger",
		c.appName, c.workerName, c.debugTCPPort, c.osPid)

	c.consumerHandle.sendMsgToDebugger = false

	c.consumerHandle.debugListener.Close()

	if c.osPid > 1 {
		ps, err := os.FindProcess(c.osPid)
		if err == nil {
			ps.Kill()
		}
	}

}

func (c *debugClient) String() string {
	return fmt.Sprintf("consumer_debug_client => app: %s workerName: %s tcpPort: %s ospid: %d",
		c.appName, c.workerName, c.debugTCPPort, c.osPid)
}

func (c *Consumer) pollForDebuggerStart() {
	dFlagKey := fmt.Sprintf("%s::%s", c.app.AppName, startDebuggerFlag)
	dInstAddrKey := fmt.Sprintf("%s::%s", c.app.AppName, debuggerInstanceAddr)

	dFlagBlob := &common.StartDebugBlob{}
	dInstAddrBlob := &common.DebuggerInstanceAddrBlob{}
	var cas gocb.Cas

	for {

		select {
		case <-c.signalStopDebuggerRoutineCh:
			return
		default:
		}

		c.debuggerState = debuggerOpcode

		util.Retry(util.NewFixedBackoff(bucketOpRetryInterval), getOpCallback, c, dFlagKey, dFlagBlob, &cas, false)
		if !dFlagBlob.StartDebug {
			time.Sleep(debuggerFlagCheckInterval)
			continue
		} else {
			c.debuggerState = startDebug

			// In case some other Eventing.Consumer instance starts the debugger, below
			// logic keeps an eye on startDebugger blob in metadata bucket and calls continue
			stopBucketLookupRoutineCh := make(chan struct{}, 1)

			go func(c *Consumer, stopBucketLookupRoutineCh chan struct{}) {
				for {
					time.Sleep(time.Second)

					select {
					case <-stopBucketLookupRoutineCh:
						return
					default:
					}
					dFlagKey := fmt.Sprintf("%s::%s", c.app.AppName, startDebuggerFlag)
					dFlagBlob := &common.StartDebugBlob{}

					util.Retry(util.NewFixedBackoff(bucketOpRetryInterval), getOpCallback, c, dFlagKey, dFlagBlob, &cas, false)
					if !dFlagBlob.StartDebug {
						c.signalDebugBlobDebugStopCh <- struct{}{}
						return
					}

				}
			}(c, stopBucketLookupRoutineCh)

			select {
			case <-c.signalDebugBlobDebugStopCh:
				continue
			case <-c.signalUpdateDebuggerInstBlobCh:
				stopBucketLookupRoutineCh <- struct{}{}
			}

		checkDInstAddrBlob:
			util.Retry(util.NewFixedBackoff(bucketOpRetryInterval), getOpCallback, c, dInstAddrKey, dInstAddrBlob, &cas, false)

			logging.Infof("CRPS[%s:%s:%s:%d] Debugger inst addr key: %s dump: %#v",
				c.app.AppName, c.ConsumerName(), c.debugTCPPort, c.Pid(), dInstAddrKey, dInstAddrBlob)

			if dInstAddrBlob.HostPortAddr == "" {

				dInstAddrBlob.ConsumerName = c.ConsumerName()
				dInstAddrBlob.HostPortAddr = c.HostPortAddr()
				dInstAddrBlob.NodeUUID = c.NodeUUID()

				_, err := c.gocbMetaBucket.Replace(dInstAddrKey, dInstAddrBlob, gocb.Cas(cas), 0)
				if err != nil {
					logging.Errorf("CRPS[%s:%s:%s:%d] Bucket cas failed for debugger inst addr key: %s, err: %v",
						c.app.AppName, c.ConsumerName(), c.debugTCPPort, c.Pid(), dInstAddrKey, err)
					goto checkDInstAddrBlob
				} else {
					dFlagBlob.StartDebug = false
					util.Retry(util.NewFixedBackoff(bucketOpRetryInterval), setOpCallback, c, dFlagKey, dFlagBlob)

					c.signalStartDebuggerCh <- struct{}{}
				}
			}
			c.signalInstBlobCasOpFinishCh <- struct{}{}
		}
	}
}

func (c *Consumer) startDebuggerServer() {
	var err error
	c.debugListener, err = net.Listen("tcp", ":0")
	if err != nil {
		logging.Errorf("CRSD[%s:%s:%s:%d] Failed to listen while trying to start communication to C++ debugger, err: %v",
			c.app.AppName, c.ConsumerName(), c.debugTCPPort, c.Pid(), err)
		return
	}

	logging.Infof("CRSD[%s:%s:%s:%d] Start server on addr: %v for communication to C++ debugger",
		c.app.AppName, c.ConsumerName(), c.tcpPort, c.Pid(), c.debugListener.Addr().String())

	c.signalDebuggerConnectedCh = make(chan struct{}, 1)

	go func(c *Consumer) {
		for {
			c.debugConn, err = c.debugListener.Accept()
			c.signalDebuggerConnectedCh <- struct{}{}
		}
	}(c)

	c.debugTCPPort = strings.Split(c.debugListener.Addr().String(), ":")[3]
	c.debugClient = newDebugClient(c, c.app.AppName, c.eventingAdminPort, c.debugTCPPort, c.workerName)
	c.debugClientSupToken = c.consumerSup.Add(c.debugClient)

	<-c.signalDebuggerConnectedCh

	c.sendLogLevel(c.logLevel, true)

	partitions := make([]uint16, cppWorkerPartitionCount)
	for i := 0; i < int(cppWorkerPartitionCount); i++ {
		partitions[i] = uint16(i)
	}
	thrPartitionMap := util.VbucketDistribution(partitions, 1)
	c.sendWorkerThrMap(thrPartitionMap, true)

	c.sendWorkerThrCount(1, true) // Spawn just one thread when debugger is spawned to avoid complexity

	util.Retry(util.NewFixedBackoff(clusterOpRetryInterval), getEventingNodeAddrOpCallback, c)

	var currHost string
	h := c.HostPortAddr()
	if h != "" {
		currHost = strings.Split(h, ":")[0]
	} else {
		currHost = "127.0.0.1"
	}

	var user, password string
	util.Retry(util.NewFixedBackoff(time.Second), getMemcachedServiceAuth, c.producer.KvHostPorts()[0], &user, &password)

	payload := makeV8InitPayload(c.app.AppName, currHost, c.eventingDir, c.eventingAdminPort,
		c.producer.KvHostPorts()[0], c.producer.CfgData(), user, password, c.lcbInstCapacity,
		c.executionTimeout, c.enableRecursiveMutation)
	logging.Debugf("CRSD[%s:%s:%s:%d] Debug enabled V8 worker init enable_recursive_mutation flag: %v",
		c.app.AppName, c.workerName, c.debugTCPPort, c.Pid(), c.enableRecursiveMutation)

	c.sendInitV8Worker(payload, true)

	c.sendDebuggerStart()

	c.sendLoadV8Worker(c.app.AppCode, true)

	c.debuggerStarted = true
}

func (c *Consumer) stopDebuggerServer() {
	logging.Infof("CRSD[%s:%s:%s:%d] Closing connection to C++ worker for debugger. Local addr: %v, remote addr: %v",
		c.app.AppName, c.ConsumerName(), c.debugTCPPort, c.Pid(), c.debugConn.LocalAddr().String(), c.debugConn.RemoteAddr().String())

	c.debuggerStarted = false
	c.sendMsgToDebugger = false

	c.debugConn.Close()
	c.debugListener.Close()
}
