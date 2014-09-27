package nds

import (
	"reflect"

	"appengine"
	"appengine/datastore"
	"appengine/memcache"
)

// putMultiLimit is the App Engine datastore limit for the maximum number
// of entities that can be put by the datastore.PutMulti at once.
const putMultiLimit = 500

// PutMulti works just like datastore.PutMulti except it interacts
// appropriately with NDS's caching strategy.
// vals can only be slices of structs, []S.
func PutMulti(c appengine.Context,
	keys []*datastore.Key, vals interface{}) ([]*datastore.Key, error) {

	if err := checkMultiArgs(keys, reflect.ValueOf(vals)); err != nil {
		return nil, err
	}

	return putMulti(c, keys, vals)
}

func Put(c appengine.Context,
	key *datastore.Key, val interface{}) (*datastore.Key, error) {

	if err := checkArgs(key, val); err != nil {
		return nil, err
	}

	keys, err := putMulti(c, []*datastore.Key{key}, []interface{}{val})
	if me, ok := err.(appengine.MultiError); ok {
		return nil, me[0]
	} else if err != nil {
		return nil, err
	}
	return keys[0], nil
}

// putMulti puts the entities into the datastore and then its local cache.
func putMulti(c appengine.Context,
	keys []*datastore.Key, vals interface{}) ([]*datastore.Key, error) {

	lockMemcacheKeys := make([]string, 0, len(keys))
	lockMemcacheItems := make([]*memcache.Item, 0, len(keys))
	for _, key := range keys {
		if !key.Incomplete() {
			item := &memcache.Item{
				Key:        createMemcacheKey(key),
				Flags:      lockItem,
				Value:      itemLock(),
				Expiration: memcacheLockTime,
			}
			lockMemcacheItems = append(lockMemcacheItems, item)
			lockMemcacheKeys = append(lockMemcacheKeys, item.Key)
		}
	}

	if err := memcache.SetMulti(c, lockMemcacheItems); err != nil {
		return nil, err
	}

	// Save to the datastore.
	keys, err := datastore.PutMulti(c, keys, vals)
	if err != nil {
		return nil, err
	}

	if !inTransaction(c) {
		// Remove the locks.
		if err := memcache.DeleteMulti(c, lockMemcacheKeys); err != nil {
			c.Warningf("putMulti memcache.DeleteMulti %s", err)
		}
	}
	return keys, nil
}
