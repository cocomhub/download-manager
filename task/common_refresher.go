package task

import "time"

type CommonRefresher struct {
	interval time.Duration
	stop     chan struct{}
}

func NewCommonRefresher(sec int) *CommonRefresher {
	if sec <= 0 {
		sec = 3600
	}
	return &CommonRefresher{
		interval: time.Duration(sec) * time.Second,
		stop:     make(chan struct{}),
	}
}

func (r *CommonRefresher) Start(fn func()) {
	go func() {
		fn()
		for {
			select {
			case <-r.stop:
				return
			case <-time.NewTimer(r.interval).C:
				fn()
			}
		}
	}()
}

func (r *CommonRefresher) UpdateInterval(sec int) {
	if sec <= 0 {
		sec = 3600
	}
	r.interval = time.Duration(sec) * time.Second
}

func (r *CommonRefresher) Stop() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
}
