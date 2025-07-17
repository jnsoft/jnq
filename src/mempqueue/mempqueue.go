package mempqueue

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"log"
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
	CHECKPOINT_COUNT    = 10_000
	DELETEME_SUFFIX     = ".deleteme"
	TRY_RESET_INTERVAL  = 10 * time.Minute
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
	pqs            []pqueue.PriorityQueue[pqItem]
	not_before_pq  pqueue.PriorityQueue[notBeforeItem]
	reserved       map[string]reservedItem
	isMinQueue     bool
	mu             sync.Mutex
	snapshotFile   string
	walFile        string
	opCount        int
	lastResetCheck time.Time
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
	pq.lastResetCheck = time.Now()
	if err := pq.load(); err != nil {
		panic("failed to load persistent queue: " + err.Error())
	}
	return pq
}

// Operations

func (pq *MemPQueue) IsEmpty(channel int) (bool, error) {
	if channel < 0 || channel >= MAX_CHANNEL {
		return true, nil
	}
	pq.processNotBeforeQueue()
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return pq.pqs[channel].IsEmpty(), nil
}

func (pq *MemPQueue) Size(channel int) (int, error) {
	if channel < 0 || channel >= MAX_CHANNEL {
		return 0, nil
	}
	pq.processNotBeforeQueue()
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return pq.pqs[channel].Size(), nil
}

func (pq *MemPQueue) Peek(channel int) (string, error) {
	if channel < 0 || channel >= MAX_CHANNEL {
		return "", errors.New(INVALID_CHANNEL_MSG)
	}
	pq.processNotBeforeQueue()
	pq.mu.Lock()
	defer pq.mu.Unlock()
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
		if pq.snapshotFile != "" {
			err := pq.appendWAL(walOp{Op: "enqueue_notbefore", Channel: channel, Item: pqItem, Time: time.Now()})
			if err != nil {
				log.Printf("Error appending to WAL: %v", err)
				return err
			} else {

			}
		}

		pq.not_before_pq.Enqueue(notBeforeItem{
			Item:    pqItem,
			Channel: channel,
		})

		pq.maybeCheckpoint()

	} else {

		if pq.snapshotFile != "" {
			err := pq.appendWAL(walOp{Op: "enqueue", Channel: channel, Item: pqItem, Time: time.Now()})
			if err != nil {
				log.Printf("Error appending to WAL: %v", err)
				return err
			}
		}

		pq.pqs[channel].Enqueue(pqItem)

		pq.maybeCheckpoint()
	}

	return nil
}

func (pq *MemPQueue) Dequeue(channel int) (string, error) {
	if channel < 0 || channel >= MAX_CHANNEL {
		return "", errors.New(INVALID_CHANNEL_MSG)
	}

	pq.processNotBeforeQueue()

	pq.mu.Lock()
	defer pq.mu.Unlock()

	item, err := pq.pqs[channel].Dequeue()
	if err != nil {
		return "", err
	}

	if pq.snapshotFile != "" {
		err := pq.appendWAL(walOp{Op: "dequeue", Channel: channel, Item: item, Time: time.Now()})
		if err != nil {
			log.Printf("Error appending to WAL: %v", err)
			pq.pqs[channel].Enqueue(item)
			return "", err

		}
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
	pq.processNotBeforeQueue()

	pq.mu.Lock()
	defer pq.mu.Unlock()

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
		err := pq.appendWAL(walOp{Op: "dequeueWithReservation", Channel: channel, Item: item, ResId: reservationId, Time: time.Now()})
		if err != nil {
			log.Printf("Error appending to WAL: %v", err)
			pq.pqs[channel].Enqueue(item)
			delete(pq.reserved, reservationId)
			return "", "", err
		}
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

	if pq.snapshotFile != "" {
		err := pq.appendWAL(walOp{Op: "confirm", ResId: reservationId, Time: time.Now()})
		if err != nil {
			log.Printf("Error appending to WAL: %v", err)
			return false, err
		}
	}

	delete(pq.reserved, reservationId)
	pq.maybeCheckpoint()
	return true, nil
}

func (pq *MemPQueue) RequeueExpiredReservations(timeout time.Duration) (int, error) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	c := 0
	now := time.Now()
	for reservationId, reserved := range pq.reserved {
		if now.Sub(reserved.Timestamp) > timeout {
			if pq.snapshotFile != "" {
				err := pq.appendWAL(walOp{Op: "delete_reserved", Channel: reserved.Channel, Item: reserved.Item, Time: time.Now()})
				if err != nil {
					return c, err
				}
				err = pq.appendWAL(walOp{Op: "enqueue", Channel: reserved.Channel, Item: reserved.Item, Time: time.Now()})
				if err != nil {
					return c, err
				}
			}

			pq.pqs[reserved.Channel].Enqueue(reserved.Item)
			delete(pq.reserved, reservationId)
			c++
			pq.maybeCheckpoint()
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

func (pq *MemPQueue) processNotBeforeQueue() {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	for {
		if pq.not_before_pq.IsEmpty() {
			return
		}
		notBeforeItem, err := pq.not_before_pq.Peek()
		if err != nil {
			log.Printf("Error peeking not_before_pq in processNotBefore: %v", err)
			return
		}
		if time.Now().Before(notBeforeItem.Item.Not_before) {
			return

		}
		notBeforeItem, err = pq.not_before_pq.Dequeue()
		if err != nil {
			log.Printf("Error dequeueing not_before_pq in processNotBefore: %v", err)
			return
		}

		if pq.snapshotFile != "" {
			err := pq.appendWAL(walOp{Op: "enqueue", Channel: notBeforeItem.Channel, Item: notBeforeItem.Item, Time: time.Now()})
			if err != nil {
				log.Printf("Error appending to WAL: %v", err)
				pq.not_before_pq.Enqueue(notBeforeItem)
				return
			}
		}
		pq.pqs[notBeforeItem.Channel].Enqueue(notBeforeItem.Item)
		pq.maybeCheckpoint()
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
	if pq.snapshotFile != "" {
		if pq.opCount >= CHECKPOINT_COUNT {
			if err := pq.save(); err != nil {
				log.Printf("Error checking checkpoint: %v", err)
				return err
			}
			if err := os.Remove(pq.walFile); err != nil && !os.IsNotExist(err) {
				log.Panic("Error checking checkpoint:", err)
			}
			pq.opCount = 0
		}

		now := time.Now()
		if now.Sub(pq.lastResetCheck) >= TRY_RESET_INTERVAL {
			_, err := pq.resetIfNoData()
			pq.lastResetCheck = now
			if err != nil {
				log.Printf("Error running resetIfNoData: %v", err)
			}
		}
	}
	return nil
}

func (pq *MemPQueue) resetIfNoData() (bool, error) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	for i := range pq.pqs {
		if !pq.pqs[i].IsEmpty() {
			return false, nil
		}
	}

	if !pq.not_before_pq.IsEmpty() {
		return false, nil
	}

	if len(pq.reserved) > 0 {
		return false, nil
	}

	if pq.snapshotFile != "" && pq.walFile != "" {
		if err := os.Remove(pq.snapshotFile); err != nil && !os.IsNotExist(err) {
			return false, err
		}
		if err := os.Remove(pq.walFile); err != nil && !os.IsNotExist(err) {
			return false, err
		}
	}
	err := deleteSafely(pq.walFile, pq.snapshotFile)
	if err != nil {
		log.Printf("Error deleting files safely: %v", err)
		return false, err
	}
	return true, nil
}

func deleteSafely(walfile, snapfile string) error {
	if walfile != "" && snapfile != "" {
		snapTmp := snapfile + DELETEME_SUFFIX
		walTmp := walfile + DELETEME_SUFFIX

		snapRenamed := false
		walRenamed := false

		if err := os.Rename(snapfile, snapTmp); err == nil || os.IsNotExist(err) {
			snapRenamed = true
		} else {
			return err
		}

		if err := os.Rename(walfile, walTmp); err == nil || os.IsNotExist(err) {
			walRenamed = true
		} else {
			if snapRenamed {
				err = os.Rename(snapTmp, snapfile)
				if err != nil {
					log.Panic("Failed to rename snapshot file back: ", err)
				}
				snapRenamed = false
			}
			return err
		}

		if snapRenamed && walRenamed {
			if err := os.Remove(snapTmp); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.Remove(walTmp); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
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
