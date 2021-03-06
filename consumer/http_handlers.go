package consumer

import (
	cm "github.com/couchbase/eventing/common"
	"github.com/couchbase/eventing/logging"
	"github.com/couchbase/eventing/shared"
)

// RebalanceTaskProgress reports progress to producer
func (c *Consumer) RebalanceTaskProgress() *cm.RebalanceProgress {
	progress := &cm.RebalanceProgress{}

	vbsRemainingToGiveUp := c.getVbRemainingToGiveUp()
	vbsRemainingToOwn := c.getVbRemainingToOwn()

	if len(vbsRemainingToGiveUp) > 0 || len(vbsRemainingToOwn) > 0 {
		vbsOwnedPerPlan := c.getVbsOwned()

		progress.VbsOwnedPerPlan = len(vbsOwnedPerPlan)
		progress.VbsRemainingToShuffle = len(vbsRemainingToOwn) + len(vbsRemainingToGiveUp)
	}

	return progress
}

// EventsProcessedPSec reports dcp + timer events triggered per sec
func (c *Consumer) EventsProcessedPSec() *cm.EventProcessingStats {
	pStats := &cm.EventProcessingStats{
		DcpEventsProcessedPSec:   c.dcpOpsProcessedPSec,
		TimerEventsProcessedPSec: c.timerMessagesProcessedPSec,
	}

	return pStats
}

// DcpEventsRemainingToProcess reports dcp events remaining to producer
func (c *Consumer) DcpEventsRemainingToProcess() uint64 {
	vbsTohandle := c.vbsToHandle()

	seqNos, err := shared.BucketSeqnos(c.producer.NsServerHostPort(), "default", c.bucket)
	if err != nil {
		logging.Errorf("CRVT[%s:%s:%s:%d] Failed to fetch get_all_vb_seqnos, err: %v", c.app.AppName, c.workerName, c.tcpPort, c.Pid(), err)
		return 0
	}

	var eventsProcessed, totalEvents uint64
	for _, vbno := range vbsTohandle {
		eventsProcessed += c.vbProcessingStats.getVbStat(vbno, "last_processed_seq_no").(uint64)
		totalEvents += seqNos[int(vbno)]
	}

	return totalEvents - eventsProcessed
}

// VbProcessingStats exposes consumer vb metadata to producer
func (c *Consumer) VbProcessingStats() map[uint16]map[string]interface{} {
	vbstats := make(map[uint16]map[string]interface{})
	for vbno := range c.vbProcessingStats {
		if _, ok := vbstats[vbno]; !ok {
			vbstats[vbno] = make(map[string]interface{})
		}
		assignedWorker := c.vbProcessingStats.getVbStat(vbno, "assigned_worker")
		owner := c.vbProcessingStats.getVbStat(vbno, "current_vb_owner")
		streamStatus := c.vbProcessingStats.getVbStat(vbno, "dcp_stream_status")
		seqNo := c.vbProcessingStats.getVbStat(vbno, "last_processed_seq_no")
		uuid := c.vbProcessingStats.getVbStat(vbno, "node_uuid")
		currentProcDocIDTimer := c.vbProcessingStats.getVbStat(vbno, "currently_processed_doc_id_timer").(string)
		currentProcNonDocIDTimer := c.vbProcessingStats.getVbStat(vbno, "currently_processed_non_doc_timer").(string)
		lastProcDocIDTimer := c.vbProcessingStats.getVbStat(vbno, "last_processed_doc_id_timer_event").(string)
		nextDocIDTimer := c.vbProcessingStats.getVbStat(vbno, "next_doc_id_timer_to_process").(string)
		nextNonDocIDTimer := c.vbProcessingStats.getVbStat(vbno, "next_non_doc_timer_to_process").(string)
		plasmaLastSeqNoPersist := c.vbProcessingStats.getVbStat(vbno, "plasma_last_seq_no_persisted").(uint64)

		vbstats[vbno]["assigned_worker"] = assignedWorker
		vbstats[vbno]["current_vb_owner"] = owner
		vbstats[vbno]["node_uuid"] = uuid
		vbstats[vbno]["stream_status"] = streamStatus
		vbstats[vbno]["seq_no"] = seqNo
		vbstats[vbno]["currently_processed_doc_id_timer"] = currentProcDocIDTimer
		vbstats[vbno]["currently_processed_non_doc_timer"] = currentProcNonDocIDTimer
		vbstats[vbno]["last_processed_doc_id_timer_event"] = lastProcDocIDTimer
		vbstats[vbno]["next_non_doc_timer_to_process"] = nextDocIDTimer
		vbstats[vbno]["next_non_doc_timer_to_process"] = nextNonDocIDTimer
		vbstats[vbno]["plasma_last_seq_no_persisted"] = plasmaLastSeqNoPersist
	}

	return vbstats
}
