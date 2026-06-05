package dicomfetch

import (
	"container/list"
	"sync"
)

type instanceCache struct {
	mu       sync.Mutex
	maxBytes int64
	curBytes int64
	ll       *list.List
	items    map[string]*list.Element
}

type cacheEntry struct {
	key      string
	instance FetchedInstance
}

func newInstanceCache(maxBytes int64) *instanceCache {
	return &instanceCache{
		maxBytes: maxBytes,
		ll:       list.New(),
		items:    make(map[string]*list.Element),
	}
}

func (c *instanceCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

func (c *instanceCache) bytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.curBytes
}

func (c *instanceCache) get(key string) (FetchedInstance, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return FetchedInstance{}, false
	}
	c.ll.MoveToFront(el) // recency update
	return el.Value.(*cacheEntry).instance, true
}

func (c *instanceCache) put(key string, instance FetchedInstance) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		ent := el.Value.(*cacheEntry)
		c.curBytes += int64(len(instance.Data)) - int64(len(ent.instance.Data))
		ent.instance = instance
		c.ll.MoveToFront(el)
	} else {
		c.items[key] = c.ll.PushFront((&cacheEntry{key: key, instance: instance}))
		c.curBytes += int64(len(instance.Data))
	}

	if c.maxBytes <= 0 {
		return
	}
	for c.curBytes > c.maxBytes && c.ll.Len() > 1 {
		c.evictOldest()
	}
}

func (c *instanceCache) evictOldest() {
	el := c.ll.Back()
	if el == nil {
		return
	}
	ent := el.Value.(*cacheEntry)
	c.ll.Remove(el)
	delete(c.items, ent.key)
	c.curBytes -= int64(len(ent.instance.Data))
}
