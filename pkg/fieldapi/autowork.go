package fieldapi

import (
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/worker"
	"emergion-sovereign-runtime/policy"
)

// NewWorker returns the nonstop worker over this runtime's store. dir holds
// the worker's on/off control and checkpoint (outside the ledger).
func (r *Runtime) NewWorker(dir string, ins reason.Insighter) *worker.Worker {
	return &worker.Worker{Store: r.store, Insighter: ins, Dir: dir}
}

// AutoStatus is the /auto/status body: policy, worker and reasoner.
type AutoStatus struct {
	Policy PolicyStatus `json:"policy"`
	Worker WorkerStatus `json:"worker"`
}

// WorkerStatus is the worker part of AutoStatus.
type WorkerStatus struct {
	On               bool              `json:"on"`
	Mode             string            `json:"mode"`
	OnlineConfigured bool              `json:"online_configured"`
	LastReasoner     string            `json:"last_reasoner,omitempty"`
	Pending          int               `json:"pending"`
	InsightRecords   int               `json:"insight_records"`
	Checkpoint       worker.Checkpoint `json:"checkpoint"`
}

// AutoStatusOf reads the ledger, the worker control and checkpoint.
func (r *Runtime) AutoStatusOf(w *worker.Worker, sw *reason.SwitchInsighter) (AutoStatus, error) {
	st, err := r.state()
	if err != nil {
		return AutoStatus{}, err
	}
	out := AutoStatus{Policy: PolicyStatusOf(st)}
	c := w.LoadControl()
	out.Worker = WorkerStatus{On: c.On, Mode: c.Mode, Checkpoint: w.LoadCheckpoint()}
	if sw != nil {
		out.Worker.OnlineConfigured = sw.OnlineConfigured()
		out.Worker.LastReasoner = sw.LastUsed()
	}
	insights, _ := worker.Done(st)
	out.Worker.InsightRecords = len(insights)
	for id, em := range st.Accepted {
		if em.REL["source_kind"] == "SOURCE_UPLOAD" && !insights[id] && out.Worker.Checkpoint.Failures[id] < worker.MaxFailures && policy.SourceNoise(st, id) == "" {
			out.Worker.Pending++
		}
	}
	return out, nil
}
