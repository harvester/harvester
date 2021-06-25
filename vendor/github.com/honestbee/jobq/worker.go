package jobq

type worker struct {
	id        int
	workQueue <-chan *job
	stopC     chan bool
	metric    Metric
}

func (w *worker) start() {
	go func() {
		for {
			select {
			case job := <-w.workQueue:
				w.metric.IncBusyWorker()

				go job.run()

				select {
				case <-job.Done():
					w.metric.DecBusyWorker()
				case <-w.stopC:
					// worker should have a chance to notify the running job for stop.
					job.Stop()
					<-job.Done()
					close(w.stopC)
					w.metric.DecBusyWorker()
					return
				}
			case <-w.stopC:
				// when the worker were requested to stop, it uses the same channel for signaling the caller.
				close(w.stopC)
				return
			}
		}
	}()
}

func (w *worker) stop() {
	w.stopC <- true
}

func (w *worker) join() {
	<-w.stopC
}

func newWorker(id int, workQueue <-chan *job, metric Metric) *worker {
	return &worker{
		id:        id,
		workQueue: workQueue,
		stopC:     make(chan bool),
		metric:    metric,
	}
}
