package badger

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	"github.com/dgraph-io/badger/v4"
)

// todo: consider moving it as the same as badger-raft
// eg. default, raft_default
type VersionedBadgerRepo struct {
	*BadgerRepo
}

// NewDefault implementation, simple KV store
func NewVersioned(db *badger.DB, tableName string, refSize int) *VersionedBadgerRepo {
	return &VersionedBadgerRepo{
		BadgerRepo: &BadgerRepo{
			db:        db,
			TableName: tableName,
			RefSize:   refSize,
		},
	}
}

func (c *VersionedBadgerRepo) Post(ctx context.Context, namespace string, refIDs []string, ID string, data content.Data) (content.Data, error) {
	// todo: other validation
	if len(refIDs) != c.RefSize {
		return content.Data{}, fmt.Errorf("invalid ref size")
	}
	if ID == "" {
		return content.Data{}, fmt.Errorf("need ID")
	}

	// todo: other validation
	var meta content.Meta
	err := json.Unmarshal(data.Meta, &meta)
	if err != nil {
		// opinionated
		return content.Data{}, fmt.Errorf("expected json mycontent meta payload: %v", string(data.Meta))
	}

	keyVsn := buildKey(true, c.TableName, namespace, refIDs, ID)

	// Key with vsn! prefix have integer value that address the actual data

	// Update the ref (version) & the actual value in a single tx
	var version uint32 // maybe we can optimze smaller value
	err = c.db.Update(func(txn *badger.Txn) error {

		prevVersion, err := c.getEntryVersion(txn, keyVsn)
		if err != nil {
			return err
		}

		// data.VersionedUseOptimisticLock

		version = prevVersion + 1

		// the actual entry value
		key := buildKey(false, c.TableName, namespace, append(refIDs, ID), strconv.FormatUint(uint64(version), 10))

		// item, err := txn.Get(key)
		// if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		// 	return err
		// }
		// if item != nil {
		// todo: maybe check equality; if equal, no need to change and return
		// }

		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], version)
		err = txn.Set(keyVsn, buf[:])
		if err != nil {
			return err
		}

		err = txn.Set(key, data.Data)
		if err != nil {
			return err
		}

		log.Printf("WRITEN KEY VERSION: %s\n", keyVsn)
		log.Printf("WRITEN VERSION: %v\n", version)
		log.Printf("WRITEN VALUE: %s\n", string(data.Data))

		// todo: save meta as well ~

		return nil
	})
	if err != nil {
		return content.Data{}, fmt.Errorf("error writing :%w", err)
	}

	return content.Data{
		Namespace: data.Namespace,
		RefIDs:    data.RefIDs,
		ID:        data.ID,
		Data:      data.Data,
		Meta:      data.Meta,
		EventID:   uint64(version),
		Version:   uint64(version),
	}, nil
}

func (c *VersionedBadgerRepo) getEntryVersion(txn *badger.Txn, keyVsn []byte) (uint32, error) {
	valueVsn, err := txn.Get(keyVsn)
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return 0, err
	}

	var version uint32
	if valueVsn != nil {
		tmp, err := valueVsn.ValueCopy(nil)
		if err != nil {
			return 0, err
		}
		version = binary.BigEndian.Uint32(tmp[:])
	}

	return version, nil
}

// Get daya by owner ID
// overwrite because we modifies the stream ( we can optimize later)
func (c *VersionedBadgerRepo) Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]content.Data, error) {
	var result = make([]content.Data, 0)

	data, err := c.Stream(ctx, namespace, refIDs, ID)
	if err != nil {
		return nil, err
	}

	for d := range data {
		result = append(result, d)
	}

	return result, nil
}

// Stream Get data
// For versioned, not the one that is "fast" (trade off of storage vs processing)
// I choose less storage so we can store many data in the server
func (c *VersionedBadgerRepo) Stream(ctx context.Context, namespace string, refIDs []string, ID string) (<-chan content.Data, error) {
	result := make(chan content.Data)

	go func() {
		defer close(result)

		itrCfg := badger.DefaultIteratorOptions
		itrCfg.AllVersions = false
		itrCfg.PrefetchValues = true
		itrCfg.Prefix = buildKey(true, c.TableName, namespace, refIDs, ID)

		log.Printf("PREFIX KEY STREAM ITER: %s\n", itrCfg.Prefix)

		err := c.db.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(itrCfg)
			defer it.Close()

			for it.Rewind(); it.Valid(); it.Next() {
				item := it.Item()
				keyVersion := item.KeyCopy(nil)

				log.Printf("> READ KEY: %s\n", keyVersion)

				// get version
				version, err := c.getEntryVersion(txn, keyVersion)
				if err != nil {
					return err
				}
				versionStr := strconv.FormatUint(uint64(version), 10)

				log.Printf("> READ KEY: %s VERSION: %v\n", keyVersion, versionStr)

				// get entry based on above version key
				keyEntry, namespace, refIDs := getEntryKeyFromVersionedKeyAndValue(keyVersion, version)

				log.Printf("> READ ENTRY KEY: %s\n", keyEntry)

				entry, err := txn.Get([]byte(keyEntry))
				if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
					return err
				}

				if entry == nil {
					// warn, entry should be there..
					continue
				}

				entryData, err := entry.ValueCopy(nil)
				if err != nil {
					return err
				}

				// build the "cosmetic"/"logical" ID from key
				var keyID string
				token := strings.Split(string(keyVersion), "::")
				if len(token) == 2 { // todo: special chars validation ofcourse for later..
					keyID = token[1]
				}

				d := content.Data{
					Namespace: namespace,
					EventID:   uint64(version),
					Version:   uint64(version),
					RefIDs:    refIDs,
					ID:        keyID, // the ID is from the "original" ID
					Data:      entryData,
				}

				log.Printf("GET KEY: %s\n", string(item.Key()))
				log.Printf("GET VALUE: %s\n", string(d.Data))

				result <- d
			}

			return nil
		})
		if err != nil {
			return
		}
	}()

	return result, nil
}

func getEntryKeyFromVersionedKeyAndValue(vk []byte, version uint32) (
	string,
	string,
	[]string,
) {
	vkey := strings.TrimPrefix(string(vk), "vsn!")
	token := strings.Split(vkey, "::")
	if len(token) != 2 {
		return "", "", nil
	}
	withoutID := token[0]
	ID := token[1]

	nsAndRefs := strings.Split(strings.Split(withoutID, "__")[1], "_")
	namespace := nsAndRefs[0]
	refIDs := nsAndRefs[1:]
	key := fmt.Sprintf("%s_%s::%v", withoutID, ID, version)
	return key, namespace, refIDs
}
