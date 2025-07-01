package mempqueue

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jnsoft/jngo/pqueue"
	"github.com/jnsoft/jnq/src/priorityqueue"
	"golang.org/x/exp/mmap"
)

const (
	MAX_CHANNEL         = 100
	INVALID_CHANNEL_MSG = "invalid channel"
	CHECKPOINT_COUNT    = 1000
)

type pqItem struct {
	Obj        string
	Prio       float64
	Not_before time.Time
}

type notBeforeItem struct {
	Item    pqItem
	Channel int
}

type reservedItem struct {
	Item      pqItem
	Channel   int
	Timestamp time.Time // Time when the item was reserved
}

type walOp struct { // Write-Ahead Log
	Op        string
	Channel   int
	Item      pqItem
	NotBefore notBeforeItem
	ResId     string
	Time      time.Time
}

type MemPQueue struct {
	pqs           []pqueue.PriorityQueue[pqItem]
	not_before_pq pqueue.PriorityQueue[notBeforeItem]
	reserved      map[string]reservedItem
	isMinQueue    bool
	mu            sync.Mutex
	snapshotFile  string
	walFile       string
	opCount       int
}

func less(i, j pqItem) bool {
	return i.Prio < j.Prio
}

func less_not_before(i, j notBeforeItem) bool {
	return i.Item.Not_before.Before(j.Item.Not_before)
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

func NewMemPQueuePersistent(IsMinQueue bool, snapshotFile, walFile string) *MemPQueue {
	pq := NewMemPQueue(IsMinQueue)
	pq.snapshotFile = snapshotFile
	pq.walFile = walFile
	pq.opCount = 0
	if err := pq.load(); err != nil {
		panic("failed to load persistent queue: " + err.Error())
	}
	return pq
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
	return item.Obj, nil
}

func (pq *MemPQueue) Enqueue(obj string, prio float64, channel int, notBefore time.Time) error {
	if channel < 0 || channel >= MAX_CHANNEL {
		return errors.New(INVALID_CHANNEL_MSG)
	}
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pqItem := pqItem{Obj: obj, Prio: prio, Not_before: notBefore}

	if !pq.isMinQueue {
		pqItem.Prio = -prio
	}

	if !notBefore.IsZero() && time.Now().Before(notBefore) {
		pq.not_before_pq.Enqueue(notBeforeItem{
			Item:    pqItem,
			Channel: channel,
		})

		if pq.snapshotFile != "" {
			pq.appendWAL(walOp{Op: "enqueue_notbefore", Channel: channel, Item: pqItem, Time: time.Now()})
			pq.maybeCheckpoint()
		}
	} else {
		pq.pqs[channel].Enqueue(pqItem)

		if pq.snapshotFile != "" {
			pq.appendWAL(walOp{Op: "enqueue", Channel: channel, Item: pqItem, Time: time.Now()})
			pq.maybeCheckpoint()
		}
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

	if pq.snapshotFile != "" {
		pq.appendWAL(walOp{Op: "dequeue", Channel: channel, Item: item, Time: time.Now()})
		pq.maybeCheckpoint()
	}

	return item.Obj, nil
}

// DequeueWithReservation dequeues an item and reserves it with a unique reservation ID.
// The reservation ID can be used to confirm the reservation later.
// returns the dequeued item and the reservation ID.
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
		Item:      item,
		Channel:   channel,
		Timestamp: time.Now(),
	}

	if pq.snapshotFile != "" {
		pq.appendWAL(walOp{Op: "dequeueWithReservation", Channel: channel, Item: item, ResId: reservationId, Time: time.Now()})
		pq.maybeCheckpoint()
	}
	return item.Obj, reservationId, nil
}

func (pq *MemPQueue) ConfirmReservation(reservationId string) (bool, error) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	_, exists := pq.reserved[reservationId]
	if !exists {
		return false, errors.New("invalid or expired reservation ID")
	}

	delete(pq.reserved, reservationId)
	if pq.snapshotFile != "" {
		pq.appendWAL(walOp{Op: "confirm", ResId: reservationId, Time: time.Now()})
		pq.maybeCheckpoint()
	}
	return true, nil
}

func (pq *MemPQueue) RequeueExpiredReservations(timeout time.Duration) (int, error) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	c := 0
	now := time.Now()
	for reservationId, reserved := range pq.reserved {
		if now.Sub(reserved.Timestamp) > timeout {
			c++
			pq.pqs[reserved.Channel].Enqueue(reserved.Item)
			delete(pq.reserved, reservationId)
			if pq.snapshotFile != "" {
				err := pq.appendWAL(walOp{Op: "delete_reserved", Channel: reserved.Channel, Item: reserved.Item, Time: time.Now()})
				if err != nil {
					return c, err
				}
				err = pq.appendWAL(walOp{Op: "enqueue", Channel: reserved.Channel, Item: reserved.Item, Time: time.Now()})
				if err != nil {
					return c, err
				}
				err = pq.maybeCheckpoint()
				if err != nil {
					return c, err
				}
			}
		}
	}
	return c, nil
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
	pq.reserved = make(map[string]reservedItem)

	if pq.snapshotFile != "" {
		if err := os.Remove(pq.snapshotFile); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Remove(pq.walFile); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}

// TODO: handle mutex properly
func (pq *MemPQueue) processNotBeforeQueue() {
	for {
		if pq.not_before_pq.IsEmpty() {
			break
		}
		notBeforeItem, err := pq.not_before_pq.Peek()
		if err != nil {
			panic(err)
		}
		if time.Now().Before(notBeforeItem.Item.Not_before) {
			break
		}
		notBeforeItem, err = pq.not_before_pq.Dequeue()
		if err != nil {
			panic(err)
		}

		pq.pqs[notBeforeItem.Channel].Enqueue(notBeforeItem.Item)

		if pq.snapshotFile != "" {
			pq.appendWAL(walOp{Op: "enqueue", Channel: notBeforeItem.Channel, Item: notBeforeItem.Item, Time: time.Now()})
			pq.maybeCheckpoint()
		}
	}
}

// persistant storage functions

func (pq *MemPQueue) appendWAL(op walOp) error {
	if pq.walFile == "" {
		return nil
	}

	f, err := os.OpenFile(pq.walFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	err = enc.Encode(op)
	if err != nil {
		return err
	}
	pq.opCount++
	return nil
}

func (pq *MemPQueue) maybeCheckpoint() error {
	if pq.opCount >= CHECKPOINT_COUNT {
		if err := pq.save(); err != nil {
			return err
		}
		if err := os.Remove(pq.walFile); err != nil && !os.IsNotExist(err) {
			return err
		}
		pq.opCount = 0
	}
	return nil
}

func (pq *MemPQueue) save() error {
	if pq.snapshotFile == "" {
		return nil
	}

	pqItems := make([][]pqItem, len(pq.pqs))
	for i := range pq.pqs {
		var items []pqItem
		for item := range pq.pqs[i].GetEnumerator() {
			items = append(items, item)
		}
		pqItems[i] = items
	}

	var notBeforeItems []notBeforeItem
	for item := range pq.not_before_pq.GetEnumerator() {
		notBeforeItems = append(notBeforeItems, item)
	}

	var reservedItems []reservedItem
	var reservedIds []string
	for id, reserved := range pq.reserved {
		reservedItems = append(reservedItems, reserved)
		reservedIds = append(reservedIds, id)
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	err := enc.Encode(pqItems)
	if err != nil {
		return err
	}
	err = enc.Encode(notBeforeItems)
	if err != nil {
		return err
	}
	err = enc.Encode(reservedItems)
	if err != nil {
		return err
	}
	err = enc.Encode(reservedIds)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(pq.snapshotFile, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(buf.Bytes())
	return err
}

func (pq *MemPQueue) load() error {
	if pq.snapshotFile != "" {
		info, err := os.Stat(pq.snapshotFile)
		if err == nil && info.Size() > 0 {
			r, err := mmap.Open(pq.snapshotFile)
			if err == nil {
				defer r.Close()
				data := make([]byte, r.Len())
				_, _ = r.ReadAt(data, 0)
				buf := bytes.NewBuffer(data)
				dec := gob.NewDecoder(buf)

				// Decode pqItems
				var pqItems [][]pqItem
				if err := dec.Decode(&pqItems); err != nil {
					return err
				}

				// Decode notBeforeItems
				var notBeforeItems []notBeforeItem
				if err := dec.Decode(&notBeforeItems); err != nil {
					return err
				}

				// Decode reserved
				var reserved []reservedItem
				if err := dec.Decode(&reserved); err != nil {
					return err
				}
				var reservedIds []string
				if err := dec.Decode(&reservedIds); err != nil {
					return err
				}

				// Rebuild pqs
				pqs := make([]pqueue.PriorityQueue[pqItem], MAX_CHANNEL)
				for i := 0; i < MAX_CHANNEL; i++ {
					q := pqueue.NewPriorityQueue(less)
					for _, item := range pqItems[i] {
						q.Enqueue(item)
					}
					pqs[i] = *q
				}
				pq.pqs = pqs

				// Rebuild not_before_pq
				nbq := pqueue.NewPriorityQueue(less_not_before)
				for _, item := range notBeforeItems {
					nbq.Enqueue(item)
				}
				pq.not_before_pq = *nbq

				// Restore reserved
				pq.reserved = make(map[string]reservedItem)
				for i := 0; i < len(reserved); i++ {
					pq.reserved[reservedIds[i]] = reserved[i]
				}
			}
		}
	}

	if pq.walFile != "" {
		info, err := os.Stat(pq.walFile)
		if err == nil && info.Size() > 0 {
			f, err := os.Open(pq.walFile)
			if err == nil {
				defer f.Close()
				dec := json.NewDecoder(f)
				for {
					var op walOp
					if err := dec.Decode(&op); err != nil {
						break
					}
					switch op.Op {
					case "enqueue":
						pq.pqs[op.Channel].Enqueue(op.Item)
					case "enqueue_notbefore":
						pq.not_before_pq.Enqueue(notBeforeItem{
							Item:    op.Item,
							Channel: op.Channel,
						})
					case "dequeue":
						// Remove one item from the queue (simulate dequeue)
						_, _ = pq.pqs[op.Channel].Dequeue()
					case "dequeueWithReservation":
						// Remove from queue and add to reserved
						item, err := pq.pqs[op.Channel].Dequeue()
						if err == nil {
							pq.reserved[op.ResId] = reservedItem{
								Item:      item,
								Channel:   op.Channel,
								Timestamp: op.Time,
							}
						}
					case "confirm":
						// Remove reservation
						delete(pq.reserved, op.ResId)
					case "delete_reserved":
						// Remove reservation by value (reserved item)
						for id, reserved := range pq.reserved {
							if reserved.Item == op.Item && reserved.Channel == op.Channel {
								delete(pq.reserved, id)
								break
							}
						}
					}
				}
			}
		}
	}
	return nil
}

// Ensure MemPQueue implements IPriorityQueue
var _ priorityqueue.IPriorityQueue = (*MemPQueue)(nil)
