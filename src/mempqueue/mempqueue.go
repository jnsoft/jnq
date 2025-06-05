package mempqueue

import (
	"errors"
	"sync"
	"time"

	"github.com/jnsoft/jnq/src/pqueue"
)

const (
	MAX_CHANNEL         = 100
	INVALID_CHANNEL_MSG = "Invalid channel"
)

type IPriorityQueue interface {
	IsEmpty(channel int) (bool, error)
	Size(channel int) (int, error)
	Peek(channel int) (string, error)
	Enqueue(obj string, prio float64, channel int, notBefore time.Time) error
	Dequeue(channel int) (string, error)
	ResetQueue() error
}

type pqItem struct {
	obj        string
	prio       float64
	not_before time.Time
}

type notBeforeItem struct {
	item    pqItem
	channel int
}

type MemPQueue struct {
	pqs           []pqueue.PriorityQueue[pqItem]
	not_before_pq pqueue.PriorityQueue[notBeforeItem]
	isMinQueue    bool
	mu            sync.Mutex
}

func less(i, j pqItem) bool {
	return i.prio < j.prio
}

func less_not_before(i, j notBeforeItem) bool {
	return i.item.not_before.Before(j.item.not_before)
}

func NewMemPQueue(IsMinQueue bool) *MemPQueue {
	pqs := make([]pqueue.PriorityQueue[pqItem], MAX_CHANNEL)
	for i := 0; i < MAX_CHANNEL; i++ {
		pqs[i] = *pqueue.NewPriorityQueue[pqItem](less)
	}
	return &MemPQueue{
		pqs:           pqs,
		not_before_pq: *pqueue.NewPriorityQueue[notBeforeItem](less_not_before),
		isMinQueue:    IsMinQueue,
	}
}

func (pq *MemPQueue) IsEmpty(channel int) (bool, error) {
	if channel < 0 || channel >= MAX_CHANNEL {
		return true, nil
	}
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.processNotBeforeQueue()
	return pq.pqs[channel].IsEmpty(), nil
}

func (pq *MemPQueue) Size(channel int) (int, error) {
	if channel < 0 || channel >= MAX_CHANNEL {
		return 0, nil
	}
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.processNotBeforeQueue()
	return pq.pqs[channel].Size(), nil
}

func (pq *MemPQueue) Peek(channel int) (string, error) {
	if channel < 0 || channel >= MAX_CHANNEL {
		return "", errors.New(INVALID_CHANNEL_MSG)
	}
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.processNotBeforeQueue()
	item, err := pq.pqs[channel].Peek()
	if err != nil {
		return "", err
	}
	return item.obj, nil
}

func (pq *MemPQueue) Enqueue(obj string, prio float64, channel int, notBefore time.Time) error {
	if channel < 0 || channel >= MAX_CHANNEL {
		return errors.New(INVALID_CHANNEL_MSG)
	}
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pqItem := pqItem{obj: obj, prio: prio, not_before: notBefore}

	if !notBefore.IsZero() && time.Now().Before(notBefore) {
		pq.not_before_pq.Enqueue(notBeforeItem{
			item:    pqItem,
			channel: channel,
		})
	} else {
		if pq.isMinQueue {
			prio = -prio
		}
		pq.pqs[channel].Enqueue(pqItem)
	}

	return nil
}

func (pq *MemPQueue) Dequeue(channel int) (string, error) {
	if channel < 0 || channel >= MAX_CHANNEL {
		return "", errors.New(INVALID_CHANNEL_MSG)
	}
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pq.processNotBeforeQueue()

	item, err := pq.pqs[channel].Dequeue()
	if err != nil {
		return "", err
	}
	return item.obj, nil
}

func (pq *MemPQueue) ResetQueue() error {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pqs := make([]pqueue.PriorityQueue[pqItem], MAX_CHANNEL)
	for i := 0; i < MAX_CHANNEL; i++ {
		pqs[i] = *pqueue.NewPriorityQueue[pqItem](less)
	}
	pq.pqs = pqs
	pq.not_before_pq = *pqueue.NewPriorityQueue(less_not_before)

	return nil
}

func (pq *MemPQueue) processNotBeforeQueue() {
	for {
		if pq.not_before_pq.IsEmpty() {
			break
		}
		notBeforeItem, err := pq.not_before_pq.Peek()
		if err != nil {
			panic(err)
		}
		if time.Now().Before(notBeforeItem.item.not_before) {
			break
		}
		notBeforeItem, err = pq.not_before_pq.Dequeue()
		if err != nil {
			panic(err)
		}
		if pq.isMinQueue {
			pq.pqs[notBeforeItem.channel].Enqueue(notBeforeItem.item)
		} else {
			notBeforeItem.item.prio = -notBeforeItem.item.prio
			pq.pqs[notBeforeItem.channel].Enqueue(notBeforeItem.item)
		}
	}
}

// Ensure MemPQueue implements IPriorityQueue
var _ IPriorityQueue = (*MemPQueue)(nil)
