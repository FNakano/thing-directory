// Copyright 2014-2016 Fraunhofer Institute for Applied Information Technology FIT

package catalog

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// LevelDB storage
type LevelDBStorage struct {
	db *leveldb.DB
	wg sync.WaitGroup
}

func NewLevelDBStorage(dsn string, opts *opt.Options) (Storage, error) {
	url, err := url.Parse(dsn)
	if err != nil {
		return nil, err
	}

	// Open the database file
	db, err := leveldb.OpenFile(url.Path, opts)
	if err != nil {
		return nil, err
	}

	return &LevelDBStorage{db: db}, nil
}

// CRUD
func (s *LevelDBStorage) add(td *ThingDescription) error {

	bytes, err := json.Marshal(td)
	if err != nil {
		return err
	}

	found, err := s.db.Has([]byte(td.ID), nil)
	if err != nil {
		return err
	}
	if found {
		return &ConflictError{fmt.Sprintf("%s is not unique", td.ID)}
	}

	err = s.db.Put([]byte(td.ID), bytes, nil)
	if err != nil {
		return err
	}

	return nil
}

func (s *LevelDBStorage) get(id string) (*ThingDescription, error) {

	bytes, err := s.db.Get([]byte(id), nil)
	if err == leveldb.ErrNotFound {
		return nil, &NotFoundError{fmt.Sprintf("%s is not found", id)}
	} else if err != nil {
		return nil, err
	}

	var td ThingDescription
	err = json.Unmarshal(bytes, &td)
	if err != nil {
		return nil, err
	}

	return &td, nil
}

func (s *LevelDBStorage) update(id string, td *ThingDescription) error {

	bytes, err := json.Marshal(td)
	if err != nil {
		return err
	}

	found, err := s.db.Has([]byte(id), nil)
	if err != nil {
		return err
	}
	if !found {
		return &NotFoundError{fmt.Sprintf("%s is not found", id)}
	}

	err = s.db.Put([]byte(id), bytes, nil)
	if err != nil {
		return err
	}

	return nil
}

func (s *LevelDBStorage) delete(id string) error {

	err := s.db.Delete([]byte(id), nil)
	if err == leveldb.ErrNotFound {
		return &NotFoundError{fmt.Sprintf("%s is not found", id)}
	} else if err != nil {
		return err
	}

	return nil
}

func (s *LevelDBStorage) list(page int, perPage int) ([]ThingDescription, int, error) {

	total, err := s.total()
	if err != nil {
		return nil, 0, err
	}
	offset, limit, err := GetPagingAttr(total, page, perPage, MaxPerPage)
	if err != nil {
		return nil, 0, &BadRequestError{fmt.Sprintf("Unable to paginate: %s", err)}
	}

	// TODO: is there a better way to do this?
	// github.com/syndtr/goleveldb/leveldb/iterator
	devices := make([]ThingDescription, limit)
	s.wg.Add(1)
	iter := s.db.NewIterator(nil, nil)
	i := 0
	for iter.Next() {
		var td ThingDescription
		err = json.Unmarshal(iter.Value(), &td)
		if err != nil {
			return nil, 0, err
		}

		if i >= offset && i < offset+limit {
			devices[i-offset] = td
		} else if i >= offset+limit {
			break
		}
		i++
	}
	iter.Release()
	s.wg.Done()
	err = iter.Error()
	if err != nil {
		return nil, 0, err
	}

	return devices, total, nil
}

func (s *LevelDBStorage) total() (int, error) {
	c := 0
	s.wg.Add(1)
	iter := s.db.NewIterator(nil, nil)
	for iter.Next() {
		c++
	}
	iter.Release()
	s.wg.Done()
	err := iter.Error()
	if err != nil {
		return 0, err
	}
	return c, nil
}

func (s *LevelDBStorage) iterator() <-chan *ThingDescription {
	serviceIter := make(chan *ThingDescription)

	go func() {
		defer close(serviceIter)

		s.wg.Add(1)
		defer s.wg.Done()
		iter := s.db.NewIterator(nil, nil)
		defer iter.Release()

		for iter.Next() {
			var td ThingDescription
			err := json.Unmarshal(iter.Value(), &td)
			if err != nil {
				log.Printf("LevelDB Error: %s", err)
				return
			}
			serviceIter <- &td
		}

		err := iter.Error()
		if err != nil {
			log.Printf("LevelDB Error: %s", err)
		}
	}()

	return serviceIter
}

func (s *LevelDBStorage) Close() {
	s.wg.Wait()
	err := s.db.Close()
	if err != nil {
		log.Printf("Error closing storage: %s", err)
	}
	log.Println("Closed leveldb.")
}
