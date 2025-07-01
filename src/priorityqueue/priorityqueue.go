package priorityqueue

import "time"

type IPriorityQueue interface {
	IsEmpty(channel int) (bool, error)
	Size(channel int) (int, error)
	Peek(channel int) (string, error)
	Enqueue(obj string, prio float64, channel int, notBefore time.Time) error
	Dequeue(channel int) (string, error)
	ResetQueue() error
	RequeueExpiredReservations(timeout time.Duration) (int, error)
	DequeueWithReservation(channel int) (string, string, error)
	ConfirmReservation(reservationId string) (bool, error)
}
