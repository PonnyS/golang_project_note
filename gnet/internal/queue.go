package internal

import (
	"sync"
)

type Job func() error

type AsyncJobQueue struct {
	lock sync.Locker
	jobs []func() error
}

func (q *AsyncJobQueue) Push(job Job) (jobsNum int) {
	q.lock.Lock()
	q.jobs = append(q.jobs, job)
	jobsNum = len(q.jobs)
	q.lock.Unlock()
	return
}

func (q *AsyncJobQueue) ForEach() (err error) {
	q.lock.Lock()
	jobs := q.jobs
	q.jobs = nil
	q.lock.Unlock()
	for i := range jobs {
		if err = jobs[i](); err != nil {
			return err
		}
	}
	return
}

func NewAsyncJobQueue() AsyncJobQueue {
	return AsyncJobQueue{lock: Spinlock()}
}
