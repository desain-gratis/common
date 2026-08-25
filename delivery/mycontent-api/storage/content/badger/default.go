package badger

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/dgraph-io/badger/v4"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

var _ content.Repository = &BadgerRepo{}

var (
	ErrNotReady = errors.New("raft not ready")
)

// todo: consider moving it as the same as badger-raft
// eg. default, raft_default
type BadgerRepo struct {
	db        *badger.DB
	TableName string
	RefSize   int
}

// NewDefault implementation, simple KV store
func New(db *badger.DB, tableName string, refSize int) *BadgerRepo {
	return &BadgerRepo{
		db:        db,
		TableName: tableName,
		RefSize:   refSize,
	}
}

func (c *BadgerRepo) Post(ctx context.Context, namespace string, refIDs []string, ID string, data content.Data) (content.Data, error) {
	// todo: other validation
	if len(refIDs) != c.RefSize {
		return content.Data{}, fmt.Errorf("invalid ref size")
	}
	if ID == "" {
		return content.Data{}, fmt.Errorf("need ID")
	}

	// todo: other validation

	key := buildKey(false, c.TableName, namespace, refIDs, ID)
	err := c.db.Update(func(txn *badger.Txn) error {
		// todo: save meta as well ~
		return txn.Set(key, data.Data)
	})
	if err != nil {
		return content.Data{}, fmt.Errorf("error writing :%w", err)
	}

	log.Printf("WRITEN KEY: %s\n", key)
	log.Printf("WRITEN VALUE: %s\n", string(data.Data))

	return content.Data{ // todo:
		Namespace: data.Namespace,
		RefIDs:    data.RefIDs,
		ID:        data.ID,
		Data:      data.Data,
		Meta:      data.Meta,
		EventID:   data.EventID,
	}, nil
}

// Post within transaction for more advanced usecase
func (c *BadgerRepo) PostTx(tx *badger.Txn, namespace string, refIDs []string, ID string) error {
	return nil
}

// Get daya by owner ID
func (c *BadgerRepo) Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]content.Data, error) {
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

// Delete specific ID data. If no data, MUST return error
func (c *BadgerRepo) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (content.Data, error) {
	// todo: other validation
	if len(refIDs) != c.RefSize {
		return content.Data{}, fmt.Errorf("invalid ref size")
	}
	if ID == "" {
		return content.Data{}, fmt.Errorf("need ID")
	}

	// todo: other validation

	key := buildKey(false, c.TableName, namespace, refIDs, ID)

	prev := make([]byte, 0)
	err := c.db.Update(func(txn *badger.Txn) error {
		// todo: save meta as well ~
		value, err := txn.Get(key)
		if err != nil {
			return err
		}
		value.ValueCopy(prev)

		return txn.Delete(key)
	})
	if err != nil {
		return content.Data{}, fmt.Errorf("error writing :%w", err)
	}

	// todo: parse pre
	return content.Data{ // todo:
		Namespace: namespace,
		RefIDs:    refIDs,
		ID:        ID,
		Data:      prev,
		Meta:      []byte(`{}`), // todo:
		EventID:   0,            // todo:
	}, nil
}

// Stream Get data
func (c *BadgerRepo) Stream(ctx context.Context, namespace string, refIDs []string, ID string) (<-chan content.Data, error) {
	result := make(chan content.Data)

	go func() {
		defer close(result)

		itrCfg := badger.DefaultIteratorOptions
		itrCfg.AllVersions = false
		itrCfg.PrefetchValues = true
		itrCfg.Prefix = buildKey(false, c.TableName, namespace, refIDs, ID)

		log.Printf("PREFIX KEY: %s\n", itrCfg.Prefix)

		err := c.db.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(itrCfg)
			defer it.Close()

			for it.Rewind(); it.Valid(); it.Next() {
				item := it.Item()
				key := item.KeyCopy(nil)

				var keyID string
				token := strings.Split(string(key), "::")
				if len(token) == 2 { // todo: special chars validation ofcourse for later..
					keyID = token[1]
				}

				d := content.Data{
					Namespace: namespace,
					EventID:   0, // todo:
					RefIDs:    refIDs,
					ID:        keyID, // TODO: maybe can be cut out
				}
				value, err := item.ValueCopy(nil)
				if err != nil {
					// log warning todo
					log.Println("UHUY WWARNN")
					continue
				}

				d.Data = value

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

func buildKey(versioned bool, tableName, namespace string, refIDs []string, ID string) []byte {
	// + validation or non printable
	var key string

	if versioned {
		key = "vsn!"
	}

	key = key + tableName

	if namespace != "" {
		if namespace == "*" {
			namespace = ""
		}
		key = key + "__" + namespace
	}
	if len(refIDs) > 0 {
		key = key + "_"
		for _, refID := range refIDs[:len(refIDs)-1] {
			key = key + refID + "|"
		}
		key = key + refIDs[len(refIDs)-1]
	}
	if ID != "" {
		key = key + "::" + ID
	}
	// todo: with strings builder
	return []byte(key)
}
