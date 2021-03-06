package producer

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/couchbase/eventing/common"
	"github.com/couchbase/eventing/gen/flatbuf/cfg"
	"github.com/couchbase/eventing/logging"
	"github.com/couchbase/eventing/util"
)

func (p *Producer) parseDepcfg() error {
	logging.Infof("DCFG[%s] Opening up application file", p.appName)

	path := metakvAppsPath + p.appName
	cfgData, err := util.MetakvGet(path)
	if err == nil {
		config := cfg.GetRootAsConfig(cfgData, 0)

		p.app = new(common.AppConfig)
		p.app.AppCode = string(config.AppCode())
		p.app.AppName = string(config.AppName())
		p.app.AppState = fmt.Sprintf("%v", appUndeployed)
		p.app.AppVersion = util.GetHash(p.app.AppCode)
		p.app.LastDeploy = time.Now().UTC().Format("2006-01-02T15:04:05.000000000-0700")
		p.app.ID = int(config.Id())
		p.app.Settings = make(map[string]interface{})

		d := new(cfg.DepCfg)
		depcfg := config.DepCfg(d)

		var user, password string
		util.Retry(util.NewFixedBackoff(time.Second), getHTTPServiceAuth, p, &user, &password)
		p.auth = fmt.Sprintf("%s:%s", user, password)

		p.bucket = string(depcfg.SourceBucket())
		p.cfgData = string(cfgData)
		p.metadatabucket = string(depcfg.MetadataBucket())

		settingsPath := metakvAppSettingsPath + p.appName
		sData, sErr := util.MetakvGet(settingsPath)
		if sErr != nil {
			logging.Errorf("DCFG[%s] Failed to fetch settings from metakv, err: %v", p.appName, sErr)
			return sErr
		}

		settings := make(map[string]interface{})
		uErr := json.Unmarshal(sData, &settings)
		if uErr != nil {
			logging.Errorf("DCFG[%s] Failed to unmarshal settings received from metakv, err: %v", p.appName, uErr)
			return uErr
		}

		p.cleanupTimers = settings["cleanup_timers"].(bool)
		p.dcpStreamBoundary = common.DcpStreamBoundary(settings["dcp_stream_boundary"].(string))
		p.logLevel = settings["log_level"].(string)
		p.statsTickDuration = time.Duration(settings["tick_duration"].(float64))
		p.workerCount = int(settings["worker_count"].(float64))
		p.timerWorkerPoolSize = int(settings["timer_worker_pool_size"].(float64))
		p.socketWriteBatchSize = int(settings["sock_batch_size"].(float64))
		p.skipTimerThreshold = int(settings["skip_timer_threshold"].(float64))

		// TODO: Remove if exists checking once UI starts to pass below fields
		if val, ok := settings["lcb_inst_capacity"]; ok {
			p.lcbInstCapacity = int(val.(float64))
		} else {
			p.lcbInstCapacity = 5
		}

		if val, ok := settings["enable_recursive_mutation"]; ok {
			p.enableRecursiveMutation = val.(bool)
		} else {
			p.enableRecursiveMutation = false
		}

		if val, ok := settings["deadline_timeout"]; ok {
			p.socketTimeout = time.Duration(val.(float64)) * time.Second
		} else {
			p.socketTimeout = time.Duration(2 * time.Second)
		}

		if val, ok := settings["vb_ownership_giveup_routine_count"]; ok {
			p.vbOwnershipGiveUpRoutineCount = int(val.(float64))
		} else {
			p.vbOwnershipGiveUpRoutineCount = 3
		}

		if val, ok := settings["vb_ownership_takeover_routine_count"]; ok {
			p.vbOwnershipTakeoverRoutineCount = int(val.(float64))
		} else {
			p.vbOwnershipTakeoverRoutineCount = 3
		}

		if val, ok := settings["execution_timeout"]; ok {
			p.executionTimeout = int(val.(float64))
		} else {
			p.executionTimeout = 1
		}

		if val, ok := settings["cpp_worker_thread_count"]; ok {
			p.cppWorkerThrCount = int(val.(float64))
		} else {
			p.cppWorkerThrCount = 1
		}

		p.app.Settings = settings

		logLevel := settings["log_level"].(string)
		logging.SetLogLevel(util.GetLogLevel(logLevel))

		logging.Infof("DCFG[%s] Loaded app => wc: %v auth: %v bucket: %v statsTickD: %v",
			p.appName, p.workerCount, p.auth, p.bucket, p.statsTickDuration)

		if p.workerCount <= 0 {
			return fmt.Errorf("%v", errorUnexpectedWorkerCount)
		}

		hostaddr := fmt.Sprintf("127.0.0.1:%s", p.nsServerPort)

		localAddress, err := util.LocalEventingServiceHost(p.auth, hostaddr)
		if err != nil {
			logging.Errorf("DCFG[%s] Failed to get address for local eventing node, err :%v", p.appName, err)
			return err
		}

		p.kvHostPorts, err = util.KVNodesAddresses(p.auth, hostaddr)
		if err != nil {
			logging.Errorf("DCFG[%s] Failed to get list of kv nodes in the cluster, err: %v", p.appName, err)
			return err
		}

		p.nsServerHostPort = fmt.Sprintf("%s:%s", localAddress, p.nsServerPort)

	} else {
		logging.Errorf("DCFG[%s] Failed to read depcfg, err: %v", p.appName, err)
		return err
	}
	return nil
}
