package mempqueue

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jnsoft/jngo/pqueue"
	"github.com/jnsoft/jnq/src/priorityqueue"
)

const (
	MAX_CHANNEL         = 100
	INVALID_CHANNEL_MSG = "invalid channel"
)

type pqItem struct {
	obj        string
	prio       float64
	not_before time.Time
}

type notBeforeItem struct {
	item    pqItem
	channel int
}

type reservedItem struct {
	item      pqItem
	channel   int
	timestamp time.Time // Time when the item was reserved
}

type MemPQueue struct {
	pqs           []pqueue.PriorityQueue[pqItem]
	not_before_pq pqueue.PriorityQueue[notBeforeItem]
	reserved      map[string]reservedItem
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
		pqs[i] = *pqueue.NewPriorityQueue(less)
	}
	return &MemPQueue{
		pqs:           pqs,
		not_before_pq: *pqueue.NewPriorityQueue(less_not_before),
		reserved:      make(map[string]reservedItem),
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

	if pq.isMinQueue {
		pqItem.prio = -prio
	}

	if !notBefore.IsZero() && time.Now().Before(notBefore) {
		pq.not_before_pq.Enqueue(notBeforeItem{
			item:    pqItem,
			channel: channel,
		})
	} else {
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

func (pq *MemPQueue) DequeueWithReservation(channel int) (string, string, error) {
	if channel < 0 || channel >= MAX_CHANNEL {
		return "", "", errors.New(INVALID_CHANNEL_MSG)
	}
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pq.processNotBeforeQueue()

	item, err := pq.pqs[channel].Dequeue()
	if err != nil {
		return "", "", err
	}

	reservationId := uuid.New().String()
	pq.reserved[reservationId] = reservedItem{
		item:      item,
		channel:   channel,
		timestamp: time.Now(),
	}

	return item.obj, reservationId, nil
}

func (pq *MemPQueue) ConfirmReservation(reservationId string) (bool, error) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	_, exists := pq.reserved[reservationId]
	if !exists {
		return false, errors.New("invalid or expired reservation ID")
	}

	delete(pq.reserved, reservationId)
	return true, nil
}

func (pq *MemPQueue) RequeueExpiredReservations(timeout time.Duration) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	now := time.Now()
	for reservationID, reserved := range pq.reserved {
		if now.Sub(reserved.timestamp) > timeout {
			pq.pqs[reserved.channel].Enqueue(reserved.item)
			delete(pq.reserved, reservationID)
		}
	}
}

func (pq *MemPQueue) ResetQueue() error {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pqs := make([]pqueue.PriorityQueue[pqItem], MAX_CHANNEL)
	for i := 0; i < MAX_CHANNEL; i++ {
		pqs[i] = *pqueue.NewPriorityQueue(less)
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
var _ priorityqueue.IPriorityQueue = (*MemPQueue)(nil)
