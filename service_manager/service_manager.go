package servicemanager

import (
	"bytes"
	"os"

	"github.com/couchbase/cbauth/service"
	"github.com/couchbase/eventing/logging"
)

// GetNodeInfo callback for cbauth service.Manager
func (m *ServiceMgr) GetNodeInfo() (*service.NodeInfo, error) {
	logging.Debugf("SMRB ServiceMgr::GetNodeInfo s.nodeInfo: %#v", m.nodeInfo)
	return m.nodeInfo, nil
}

// Shutdown callback for cbauth service.Manager
func (m *ServiceMgr) Shutdown() error {
	logging.Debugf("SMRB ServiceMgr::Shutdown")

	os.Exit(0)

	return nil
}

// GetTaskList callback for cbauth service.Manager
func (m *ServiceMgr) GetTaskList(rev service.Revision, cancel service.Cancel) (*service.TaskList, error) {
	logging.Debugf("SMRB ServiceMgr::GetTaskList rev: %#v", rev)

	state, err := m.wait(rev, cancel)
	if err != nil {
		return nil, err
	}

	taskList := stateToTaskList(state)
	logging.Debugf("SMRB ServiceMgr::GetTaskList tasklist: %#v", taskList)

	return taskList, nil
}

// CancelTask callback for cbauth service.Manager
func (m *ServiceMgr) CancelTask(id string, rev service.Revision) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Debugf("SMRB ServiceMgr::CancelTask id: %#v rev: %#v", id, rev)

	tasks := stateToTaskList(m.state).Tasks
	task := (*service.Task)(nil)

	for i := range tasks {
		t := &tasks[i]

		if t.ID == id {
			task = t
			break
		}
	}

	if task == nil {
		return service.ErrNotFound
	}

	if !task.IsCancelable {
		return service.ErrNotSupported
	}

	if rev != nil && !bytes.Equal(rev, task.Rev) {
		return service.ErrConflict
	}

	return m.cancelActualTaskLocked(task)
}

// GetCurrentTopology callback for cbauth service.Manager
func (m *ServiceMgr) GetCurrentTopology(rev service.Revision, cancel service.Cancel) (*service.Topology, error) {
	logging.Debugf("SMRB ServiceMgr::GetCurrentTopology rev: %#v", rev)

	state, err := m.wait(rev, cancel)
	if err != nil {
		return nil, err
	}

	topology := m.stateToTopology(state)
	logging.Debugf("ServiceMgr::GetCurrentTopology topology: %#v", topology)

	return topology, nil

}

// PrepareTopologyChange callback for cbauth service.Manager
func (m *ServiceMgr) PrepareTopologyChange(change service.TopologyChange) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Debugf("SMRB ServiceMgr::PrepareTopologyChange change: %#v", change)

	var keepNodeUUIDs []string

	for _, node := range change.KeepNodes {
		keepNodeUUIDs = append(keepNodeUUIDs, string(node.NodeInfo.NodeID))
	}

	logging.Infof("SMRB ServiceMgr::PrepareTopologyChange keepNodeUUIDs: %v", keepNodeUUIDs)

	m.updateStateLocked(func(s *state) {
		m.rebalanceID = change.ID
	})

	m.superSup.NotifyPrepareTopologyChange(keepNodeUUIDs)

	return nil
}

// StartTopologyChange callback for cbauth service.Manager
func (m *ServiceMgr) StartTopologyChange(change service.TopologyChange) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Debugf("SMRB ServiceMgr::StartTopologyChange change: %#v", change)

	if m.state.rebalanceID != change.ID || m.rebalancer != nil {
		logging.Errorf("SMRB ServiceMgr::StartTopologyChange returning errConflict, rebalanceID: %v change id: %v rebalancer dump: %#v",
			m.state.rebalanceID, change.ID, m.rebalancer)
		return service.ErrConflict
	}

	if change.CurrentTopologyRev != nil {
		haveRev := decodeRev(change.CurrentTopologyRev)
		if haveRev != m.state.rev {
			logging.Errorf("SMRB ServiceMgr::StartTopologyChange returning errConflict, state rev: %v haveRev: %v",
				m.state.rev, haveRev)
			return service.ErrConflict
		}
	}

	ctx := &rebalanceContext{
		rev:    0,
		change: change,
	}

	m.rebalanceCtx = ctx

	switch change.Type {
	case service.TopologyChangeTypeFailover:
		m.failoverNotif = true
	case service.TopologyChangeTypeRebalance:
		m.startRebalance(change)

		rebalancer := newRebalancer(m.eventingAdminPort, change, m.rebalanceDoneCallback, m.rebalanceProgressCallback)
		m.rebalancer = rebalancer

	default:
		return service.ErrNotSupported
	}

	return nil
}
