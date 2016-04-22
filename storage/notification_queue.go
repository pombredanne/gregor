package storage

import (
	"github.com/keybase/gregor"
)

type notificationQueue []gregor.Notification

func (nq notificationQueue) Len() int {
	return len(nq)
}

func (nq notificationQueue) Less(i, j int) bool {
	return nq[i].NotifyTime().Before(nq[j].NotifyTime())
}

func (nq notificationQueue) Swap(i, j int) {
	nq[i], nq[j] = nq[j], nq[i]
}

func (nq *notificationQueue) Push(x interface{}) {
	*nq = append(*nq, x.(gregor.Notification))
}

func (nq *notificationQueue) Pop() interface{} {
	n := len(*nq)
	item := (*nq)[n-1]
	*nq = (*nq)[0 : n-1]
	return item
}
