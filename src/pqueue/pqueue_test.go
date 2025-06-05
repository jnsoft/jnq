package pqueue

import (
	"math/rand"
	"testing"

	. "github.com/jnsoft/jnq/src/testhelper"
)

func TestPriorityQueue(t *testing.T) {
	t.Run("integer min priority queue", func(t *testing.T) {
		q := NewPriorityQueue[int](func(i, j int) bool { return i < j })

		// check stack is empty
		AssertTrue(t, q.IsEmpty())
		AssertEqual(t, q.Size(), 0)

		// enqueue item, then check it's not empty
		q.Enqueue(3)
		q.Enqueue(1)
		q.Enqueue(2)

		AssertFalse(t, q.IsEmpty())
		AssertEqual(t, q.Size(), 3)

		// peek first item
		value, err := q.Peek()
		AssertNil(t, err)
		AssertEqual(t, value, 1)

		// dequeue first item
		value, err = q.Dequeue()
		AssertNil(t, err)
		AssertEqual(t, value, 1)

		// dequeue second item
		value, err = q.Dequeue()
		AssertNil(t, err)
		AssertEqual(t, value, 2)

		// dequeue third item, check queue is empty
		value, err = q.Dequeue()
		AssertNil(t, err)
		AssertEqual(t, value, 3)
		AssertTrue(t, q.IsEmpty())

		// can get the numbers we put in as numbers, not untyped interface{}
		q.Enqueue(1)
		q.Enqueue(2)
		fst, _ := q.Dequeue()
		scd, _ := q.Dequeue()
		AssertEqual(t, fst+scd, 3)

		// string representation
		q.Enqueue(1)
		q.Enqueue(2)
		q.Enqueue(4)
		q.Enqueue(3)
		string_rep := q.PrettyPrint()
		AssertEqual(t, string_rep, "1->2->3->4")
	})

	t.Run("float min priority queue", func(t *testing.T) {

		q := NewPriorityQueue[float64](func(i, j float64) bool { return i < j })
		r := rand.New(rand.NewSource(42))
		for i := 0; i < 100000; i++ {
			d := r.Float64()
			q.Enqueue(d)
		}

		testOk := true
		old_val := -1.0
		for !q.IsEmpty() && testOk {
			val, err := q.Dequeue()
			testOk = err == nil && val >= old_val
			old_val = val
		}
		AssertTrue(t, testOk)
	})

	t.Run("integer max priority queue", func(t *testing.T) {

		q := NewPriorityQueue[int](func(i, j int) bool { return i > j })

		// check stack is empty
		AssertTrue(t, q.IsEmpty())
		AssertEqual(t, q.Size(), 0)

		// enqueue item, then check it's not empty
		q.Enqueue(3)
		q.Enqueue(1)
		q.Enqueue(2)

		AssertFalse(t, q.IsEmpty())
		AssertEqual(t, q.Size(), 3)

		// peek first item
		value, err := q.Peek()
		AssertNil(t, err)
		AssertEqual(t, value, 3)

		// dequeue first item
		value, err = q.Dequeue()
		AssertNil(t, err)
		AssertEqual(t, value, 3)

		// dequeue second item
		value, err = q.Dequeue()
		AssertNil(t, err)
		AssertEqual(t, value, 2)

		// dequeue third item, check queue is empty
		value, err = q.Dequeue()
		AssertNil(t, err)
		AssertEqual(t, value, 1)
		AssertTrue(t, q.IsEmpty())

		// can get the numbers we put in as numbers, not untyped interface{}
		q.Enqueue(1)
		q.Enqueue(2)
		fst, _ := q.Dequeue()
		scd, _ := q.Dequeue()
		AssertEqual(t, fst+scd, 3)

		// string representation
		q.Enqueue(3)
		q.Enqueue(1)
		q.Enqueue(2)
		string_rep := q.PrettyPrint()
		AssertEqual(t, string_rep, "3->2->1")
	})

	t.Run("float max priority queue", func(t *testing.T) {

		q := NewPriorityQueue[float64](func(i, j float64) bool { return i > j })
		r := rand.New(rand.NewSource(42))
		for i := 0; i < 100000; i++ {
			d := r.Float64()
			q.Enqueue(d)
		}

		testOk := true
		old_val := 2.0
		for !q.IsEmpty() && testOk {
			val, err := q.Dequeue()
			testOk = err == nil && val <= old_val
			old_val = val
		}
		AssertTrue(t, testOk)
	})

	t.Run("complex task priority queue", func(t *testing.T) {

		type Task struct {
			prio int
			time int
			kind string
		}

		less := func(i, j Task) bool {
			if i.prio != j.prio {
				return i.prio < j.prio
			}
			if i.time != j.time {
				return i.time < j.time
			}
			return i.kind < j.kind
		}

		pq := NewPriorityQueue(less)

		tasks := []Task{
			{prio: 3, time: 10, kind: "B"},
			{prio: 1, time: 20, kind: "A"},
			{prio: 2, time: 15, kind: "C"},
			{prio: 1, time: 10, kind: "B"},
			{prio: 2, time: 15, kind: "A"},
		}

		expectedOrder := []Task{
			{prio: 1, time: 10, kind: "B"},
			{prio: 1, time: 20, kind: "A"},
			{prio: 2, time: 15, kind: "A"},
			{prio: 2, time: 15, kind: "C"},
			{prio: 3, time: 10, kind: "B"},
		}

		for _, task := range tasks {
			pq.Enqueue(task)
		}

		ix := 0
		for !pq.IsEmpty() {
			v, err := pq.Dequeue()
			AssertNil(t, err)
			AssertEqual(t, v, expectedOrder[ix])
			ix++
		}

		AssertEqual(t, ix, len(expectedOrder))
	})

	/*
		t.Run("thread safety", func(t *testing.T) {
			q := NewPriorityQueue(func(i, j int) bool { return i < j })

			var wg sync.WaitGroup
			numGoroutines := 10
			numItems := 1000

			// Enqueue items concurrently
			for i := range numGoroutines {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					for j := range numItems {
						q.Enqueue(id*numItems + j)
					}
				}(i)
			}

			// Dequeue items concurrently
			for range numGoroutines {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for range numItems {
						q.Dequeue()
					}
				}()
			}

			wg.Wait()

			// Check if the queue is empty after all operations
			AssertTrue(t, q.IsEmpty())
			AssertEqual(t, q.Size(), 0)
		})
	*/

}
