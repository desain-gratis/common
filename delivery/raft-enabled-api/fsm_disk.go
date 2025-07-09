package raftenabledapi

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	bolt "go.etcd.io/bbolt"

	sm "github.com/lni/dragonboat/v4/statemachine"
)

const (
	testDBDirName      string = "example-data"
	currentDBFilename  string = "current"
	updatingDBFilename string = "current.updating"
)

var (
	configBucket []byte = []byte("config")

	lastEchoKey     []byte = []byte("echo")
	gsiConfigKey    []byte = []byte("gsi_config")
	appliedIndexKey []byte = []byte("applied_index")
)

// DiskKV is a state machine that implements the IOnDiskStateMachine interface.
// DiskKV stores key-value pairs in the underlying PebbleDB key-value store. As
// it is used as an example, it is implemented using the most basic features
// common in most key-value stores. This is NOT a benchmark program.
type DiskKV struct {
	shardID     uint64
	replicaID   uint64
	lastApplied uint64
	db          *bolt.DB
	closed      bool
	aborted     bool
}

// NewDiskKV creates a new disk kv test state machine.
func NewDiskKV(clusterID uint64, nodeID uint64) sm.IOnDiskStateMachine {
	d := &DiskKV{
		shardID:   clusterID,
		replicaID: nodeID,
	}
	return d
}

// Open opens the state machine and return the index of the last Raft Log entry
// already updated into the state machine.
func (d *DiskKV) Open(stopc <-chan struct{}) (uint64, error) {
	path := fmt.Sprintf(".rjs/%v/state-%v.db", d.replicaID, d.shardID)
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return 0, err
	}

	var lastAppliedIdx uint64
	err = db.Update(func(tx *bolt.Tx) error {
		cb, err := tx.CreateBucketIfNotExists([]byte("state"))
		if err != nil {
			return err
		}

		val := cb.Get([]byte("appliedIndex"))
		if len(val) == 0 {
			return nil
		}

		lastAppliedIdx = binary.LittleEndian.Uint64(val)

		return nil
	})
	if err != nil {
		return 0, err
	}

	d.db = db
	d.lastApplied = lastAppliedIdx

	return d.lastApplied, nil
}

// Lookup queries the state machine.
func (d *DiskKV) Lookup(key interface{}) (interface{}, error) {
	var val []byte
	// next, validate
	// next, batch update
	err := d.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(configBucket)
		if err != nil {
			return err
		}

		val = bucket.Get(lastEchoKey)

		return nil
	})
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	return string(val) + "  waalaikumsalam", nil
}

const (
	// Allow GSI authentication in the cluster
	SetGSIConfig MessageType = "set_gsi_config"

	Echo MessageType = "echo"
)

type MessageType string

// Google Sign-in config that allows authentication to the cluster
type GSIConfig struct {
	ClientID    string `json:"client_id"`
	AdminEmails string `json:"admin_emails"`
}

// Update updates the state machine. In this example, all updates are put into
// a PebbleDB write batch and then atomically written to the DB together with
// the index of the last Raft Log entry. For simplicity, we always Sync the
// writes (db.wo.Sync=True). To get higher throughput, you can implement the
// Sync() method below and choose not to synchronize for every Update(). Sync()
// will periodically called by Dragonboat to synchronize the state.
func (d *DiskKV) Update(ents []sm.Entry) ([]sm.Entry, error) {
	if d.aborted {
		panic("update() called after abort set to true")
	}
	if d.closed {
		panic("update called after Close()")
	}

	for idx, e := range ents {
		var cmd Message

		if err := json.Unmarshal(e.Cmd, &cmd); err != nil {
			continue
		}

		switch cmd.Type {
		case Echo:
			log.Println("RECEIVE ECHO", string(cmd.Payload))
			err := d.db.Update(func(tx *bolt.Tx) error {
				bucket, err := tx.CreateBucketIfNotExists(configBucket)
				if err != nil {
					return err
				}

				var msg string
				err = json.Unmarshal(cmd.Payload, &msg)
				if err != nil {
					return err
				}

				err = bucket.Put(lastEchoKey, []byte(msg))
				if err != nil {
					return err
				}

				return nil
			})
			if err != nil {
				log.Println("RECEIVE ECHO ERROR", string(cmd.Payload))

				continue
			}
		case SetGSIConfig:
			var gsiConfig GSIConfig
			if err := json.Unmarshal(cmd.Payload, &gsiConfig); err != nil {
				continue
			}

			// next, validate
			// next, batch update
			err := d.db.Update(func(tx *bolt.Tx) error {
				bucket, err := tx.CreateBucketIfNotExists(configBucket)
				if err != nil {
					return err
				}

				err = bucket.Put(gsiConfigKey, cmd.Payload)
				if err != nil {
					return err
				}

				return nil
			})
			if err != nil {
				continue
			}

			ents[idx].Result = sm.Result{Value: uint64(len(cmd.Payload))}
		}
	}

	// save the applied index to the DB.
	appliedIndex := make([]byte, 8)
	binary.LittleEndian.PutUint64(appliedIndex, ents[len(ents)-1].Index)

	err := d.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(configBucket)
		if err != nil {
			return err
		}
		err = bucket.Put(appliedIndexKey, appliedIndex)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if d.lastApplied >= ents[len(ents)-1].Index {
		panic("lastApplied not moving forward")
	}
	d.lastApplied = ents[len(ents)-1].Index
	return ents, nil
}

// Sync synchronizes all in-core state of the state machine. Since the Update
// method in this example already does that every time when it is invoked, the
// Sync method here is a NoOP.
func (d *DiskKV) Sync() error {
	return nil
}

// PrepareSnapshot prepares snapshotting. PrepareSnapshot is responsible to
// capture a state identifier that identifies a point in time state of the
// underlying data. In this example, we use Pebble's snapshot feature to
// achieve that.
func (d *DiskKV) PrepareSnapshot() (interface{}, error) {
	return nil, nil
}

// SaveSnapshot saves the state machine state identified by the state
// identifier provided by the input ctx parameter. Note that SaveSnapshot
// is not suppose to save the latest state.
func (d *DiskKV) SaveSnapshot(ctx interface{},
	w io.Writer, done <-chan struct{}) error {

	return nil
}

// RecoverFromSnapshot recovers the state machine state from snapshot. The
// snapshot is recovered into a new DB first and then atomically swapped with
// the existing DB to complete the recovery.
func (d *DiskKV) RecoverFromSnapshot(r io.Reader,
	done <-chan struct{}) error {

	return nil
}

// Close closes the state machine.
func (d *DiskKV) Close() error {
	err := d.db.Close()
	if err != nil {
		return err
	}

	d.closed = true

	return nil
}

type Message struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}
