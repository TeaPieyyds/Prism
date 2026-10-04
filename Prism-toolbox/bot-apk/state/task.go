package state

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

type TaskType string

const (
	TaskImport TaskType = "import"
	TaskExport TaskType = "export"
	TaskMapArt TaskType = "mapart"
	TaskMedia  TaskType = "media"
)

type TaskStatus string

const (
	StatusRunning TaskStatus = "running"
	StatusPaused  TaskStatus = "paused"
	StatusDone    TaskStatus = "done"
	StatusFailed  TaskStatus = "failed"
)

// SSEEmitter is the interface that SSEHub satisfies for emitting events
type SSEEmitter interface {
	Emit(event, data string)
}

type TaskController struct {
	Hub     SSEEmitter
	task    *Checkpoint
	stopCh  chan struct{}
	mu      sync.Mutex
	running bool
}

func NewTaskController(hub SSEEmitter) *TaskController {
	return &TaskController{
		Hub:     hub,
		stopCh:  make(chan struct{}),
		running: true,
	}
}

func (tc *TaskController) Running() bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.running
}

func (tc *TaskController) StopCh() <-chan struct{} {
	return tc.stopCh
}

func (tc *TaskController) Stop() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if tc.running {
		close(tc.stopCh)
		tc.running = false
	}
}

func (tc *TaskController) Run(taskType string, params json.RawMessage) {
	tc.mu.Lock()
	tc.running = true
	tc.stopCh = make(chan struct{})
	tc.mu.Unlock()

	taskID := uuid.New().String()
	tc.task = &Checkpoint{
		TaskID:    taskID,
		Type:      taskType,
		Status:    string(StatusRunning),
		Progress:  0,
		CreatedAt: time.Now().Unix(),
	}

	tc.Hub.Emit("task_start", mustJSON(map[string]any{"task_id": taskID, "type": taskType}))
	tc.Hub.Emit("task_done", mustJSON(map[string]any{"task_id": taskID}))
}

func (tc *TaskController) Resume(cp *Checkpoint) {
	tc.mu.Lock()
	tc.running = true
	tc.stopCh = make(chan struct{})
	tc.task = cp
	tc.mu.Unlock()

	cp.Status = string(StatusRunning)
	SaveCheckpoint(cp)
	tc.Hub.Emit("task_resume", mustJSON(map[string]any{"task_id": cp.TaskID}))
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
