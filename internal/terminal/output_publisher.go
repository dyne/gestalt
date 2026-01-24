package terminal

import (
	"context"
	"sync"
	"sync/atomic"

	"gestalt/internal/event"
)

type OutputBackpressurePolicy int

const (
	OutputBackpressureBlock OutputBackpressurePolicy = iota
	OutputBackpressureDropOldest
	OutputBackpressureDropNewest
	OutputBackpressureSample
	OutputBackpressureGrow
)

const defaultOutputQueueSize = 64
const growOutputQueueSize = 512

// OutputPublisher fans out terminal output to loggers, buffers, and subscribers.
type OutputPublisher struct {
	input         chan []byte
	done          chan struct{}
	closeOnce     sync.Once
	closed        uint32
	dropped       uint64
	policy        OutputBackpressurePolicy
	sampleEvery   uint64
	sampleCounter uint64
	logger        *SessionLogger
	buffer        *OutputBuffer
	bus           *event.Bus[[]byte]
}

type OutputPublisherOptions struct {
	Logger      *SessionLogger
	Buffer      *OutputBuffer
	Bus         *event.Bus[[]byte]
	Policy      OutputBackpressurePolicy
	MaxQueue    int
	SampleEvery uint64
}

func NewOutputPublisher(options OutputPublisherOptions) *OutputPublisher {
	maxQueue := options.MaxQueue
	if maxQueue <= 0 {
		maxQueue = defaultOutputQueueSize
	}
	if options.Policy == OutputBackpressureGrow && options.MaxQueue <= 0 {
		maxQueue = growOutputQueueSize
	}
	publisher := &OutputPublisher{
		input:       make(chan []byte, maxQueue),
		done:        make(chan struct{}),
		policy:      options.Policy,
		sampleEvery: options.SampleEvery,
		logger:      options.Logger,
		buffer:      options.Buffer,
		bus:         options.Bus,
	}
	go publisher.run()
	return publisher
}

func (p *OutputPublisher) PublishWithContext(ctx context.Context, chunk []byte) {
	if p == nil || len(chunk) == 0 {
		return
	}
	if atomic.LoadUint32(&p.closed) == 1 {
		return
	}

	switch p.policy {
	case OutputBackpressureDropOldest:
		p.publishDropOldest(chunk)
	case OutputBackpressureDropNewest:
		p.publishDropNewest(chunk)
	case OutputBackpressureSample:
		p.publishSample(chunk)
	case OutputBackpressureGrow:
		p.publishBlock(ctx, chunk)
	default:
		p.publishBlock(ctx, chunk)
	}
}

func (p *OutputPublisher) Close() {
	if p == nil {
		return
	}
	p.closeOnce.Do(func() {
		atomic.StoreUint32(&p.closed, 1)
		close(p.input)
		<-p.done
	})
}

func (p *OutputPublisher) publishBlock(ctx context.Context, chunk []byte) {
	if ctx == nil {
		p.input <- chunk
		return
	}
	select {
	case p.input <- chunk:
	case <-ctx.Done():
	}
}

func (p *OutputPublisher) publishDropOldest(chunk []byte) {
	select {
	case p.input <- chunk:
	default:
		select {
		case <-p.input:
			atomic.AddUint64(&p.dropped, 1)
		default:
		}
		select {
		case p.input <- chunk:
		default:
			atomic.AddUint64(&p.dropped, 1)
		}
	}
}

func (p *OutputPublisher) publishDropNewest(chunk []byte) {
	select {
	case p.input <- chunk:
	default:
		atomic.AddUint64(&p.dropped, 1)
	}
}

func (p *OutputPublisher) publishSample(chunk []byte) {
	if p.sampleEvery == 0 {
		p.sampleEvery = 10
	}
	counter := atomic.AddUint64(&p.sampleCounter, 1)
	if counter%p.sampleEvery != 0 {
		atomic.AddUint64(&p.dropped, 1)
		return
	}
	p.publishDropOldest(chunk)
}

func (p *OutputPublisher) run() {
	defer close(p.done)

	for chunk := range p.input {
		if p.logger != nil {
			p.logger.Write(chunk)
		}
		if p.buffer != nil {
			p.buffer.Append(chunk)
		}
		if p.bus != nil {
			p.bus.Publish(chunk)
		}
	}

	if p.bus != nil {
		p.bus.Close()
	}
	if p.logger != nil {
		_ = p.logger.Close()
	}
}
