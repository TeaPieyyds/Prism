package state

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var db *bolt.DB

const bucketName = "checkpoints"

// 节流控制：SaveCheckpoint 在 flushInterval 内只执行一次 BoltDB 写入，
// 避免大规模导入时频繁的 db.Update() 导致锁竞争和磁盘 I/O 激增。
const flushInterval = 2 * time.Second

var (
	throttleMu     sync.Mutex
	lastFlushTime  time.Time
	pendingThrottle *Checkpoint
)

func InitCheckpointDB(dataDir string) error {
	var err error
	db, err = bolt.Open(dataDir+"/td_state.db", 0644, nil)
	if err != nil {
		return fmt.Errorf("InitCheckpointDB: %w", err)
	}
	return db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		return err
	})
}

func CloseDB() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

type Checkpoint struct {
	TaskID     string          `json:"task_id"`
	Type       string          `json:"type"`
	Status     string          `json:"status"`
	Progress   float64         `json:"progress"`
	Message    string          `json:"message"`
	Data       json.RawMessage `json:"data"`   // 恢复位置
	Name       string          `json:"name"`   // 显示名称
	Params     json.RawMessage `json:"params"` // 原始任务参数(路径/坐标/速度等)
	ServerCode string          `json:"server_code"` // 绑定的服务器号
	CreatedAt  int64           `json:"created_at"`
	UpdatedAt  int64           `json:"updated_at"`
}

// SaveCheckpoint 保存断点，带节流控制。
// 终端状态（done/failed/cancelled）立即写入；进度更新每 flushInterval 合并写入一次。
// 调用方无需感知节流逻辑。
func SaveCheckpoint(cp *Checkpoint) error {
	isTerminal := cp.Status == "done" || cp.Status == "failed" || cp.Status == "cancelled"
	if isTerminal {
		// 终端状态：先刷新挂起的进度更新，再立即写入当前状态
		flushPending()
		return saveCheckpointNow(cp)
	}

	throttleMu.Lock()
	pendingThrottle = cp
	elapsed := time.Since(lastFlushTime)
	throttleMu.Unlock()

	if elapsed >= flushInterval {
		return flushPending()
	}
	return nil
}

// SaveCheckpointNow 立即写入 BoltDB，不节流。
// 用于需要确保数据持久化的场景（如创建断点、恢复前）。
func SaveCheckpointNow(cp *Checkpoint) error {
	return saveCheckpointNow(cp)
}

// FlushCheckpoints 立即将所有挂起的节流写入刷新到 BoltDB。
// 在程序退出或任务结束前调用。
func FlushCheckpoints() {
	flushPending()
}

func saveCheckpointNow(cp *Checkpoint) error {
	cp.UpdatedAt = time.Now().Unix()
	data, err := json.Marshal(cp)
	if err != nil {
		return err
	}
	return db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketName)).Put([]byte(cp.TaskID), data)
	})
}

func flushPending() error {
	throttleMu.Lock()
	cp := pendingThrottle
	pendingThrottle = nil
	lastFlushTime = time.Now()
	throttleMu.Unlock()

	if cp == nil {
		return nil
	}
	return saveCheckpointNow(cp)
}

func LoadCheckpoint(taskID string) (*Checkpoint, error) {
	var cp *Checkpoint
	err := db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte(bucketName)).Get([]byte(taskID))
		if data == nil {
			return fmt.Errorf("checkpoint not found: %s", taskID)
		}
		return json.Unmarshal(data, &cp)
	})
	return cp, err
}

func ListCheckpoints() ([]*Checkpoint, error) {
	var result []*Checkpoint
	err := db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketName)).ForEach(func(k, v []byte) error {
			var cp Checkpoint
			if err := json.Unmarshal(v, &cp); err != nil {
				return err
			}
			result = append(result, &cp)
			return nil
		})
	})
	return result, err
}

func ListCheckpointsByServer(serverCode string) ([]*Checkpoint, error) {
	var result []*Checkpoint
	err := db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketName)).ForEach(func(k, v []byte) error {
			var cp Checkpoint
			if err := json.Unmarshal(v, &cp); err != nil {
				return err
			}
			if serverCode == "" || cp.ServerCode == serverCode {
				result = append(result, &cp)
			}
			return nil
		})
	})
	return result, err
}

func DeleteCheckpoint(taskID string) error {
	return db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketName)).Delete([]byte(taskID))
	})
}