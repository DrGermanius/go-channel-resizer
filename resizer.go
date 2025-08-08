package resizer

import (
	"context"
	"runtime"
	"time"
	"unsafe"
)

type g struct{}
type lockRankStruct struct{}
type mutex struct {
	lockRankStruct
	key uintptr
}

type waitq struct {
	first *sudog
	last  *sudog
}

type sudog struct {
	g           *g
	next        *sudog
	prev        *sudog
	elem        unsafe.Pointer
	acquiretime int64
	releasetime int64
	ticket      uint32
	isSelect    bool
	success     bool
	waiters     uint16
	parent      *sudog
	waitlink    *sudog
	waittail    *sudog
	c           *hchan
}

type hchan struct {
	qcount   uint
	dataqsiz uint
	buf      unsafe.Pointer
	elemsize uint16
	closed   uint32
	elemtype uint
	sendx    uint
	recvx    uint
	recvq    waitq
	sendq    waitq
	lock     mutex
}

//go:linkname lock runtime.lock
func lock(l *mutex)

//go:linkname unlock runtime.unlock
func unlock(l *mutex)

//go:linkname goready runtime.goready
func goready(gp *g, traceskip int32)

func (q *waitq) dequeue() *sudog {
	sg := q.first
	if sg == nil {
		return nil
	}
	n := sg.next
	q.first = n
	if n == nil {
		q.last = nil
	} else {
		n.prev = nil
	}
	sg.next, sg.prev = nil, nil
	return sg
}

func Resize[T any](ch *chan T, s uint, ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	p := (*hchan)(*(*unsafe.Pointer)(unsafe.Pointer(ch)))
	sleep := 50 * time.Microsecond
	for {
		lock(&p.lock)
		old := p.dataqsiz
		if old == s {
			unlock(&p.lock)
			return nil
		}
		if s > old {
			var newBuf unsafe.Pointer
			if s > 0 {
				b := make([]T, s)
				if p.qcount > 0 && old > 0 {
					oldB := unsafe.Slice((*T)(p.buf), old)
					for i := uint(0); i < p.qcount; i++ {
						b[i] = oldB[(p.recvx+i)%old]
					}
				}
				newBuf = unsafe.Pointer(unsafe.SliceData(b))
			}
			p.buf = newBuf
			p.dataqsiz = s
			p.recvx = 0
			if s > 0 {
				p.sendx = p.qcount % s
			} else {
				p.sendx = 0
			}
			if s > 0 && p.buf != nil {
				data := unsafe.Slice((*T)(p.buf), p.dataqsiz)
				for p.qcount < p.dataqsiz {
					sg := p.sendq.dequeue()
					if sg == nil {
						break
					}
					data[p.sendx] = *(*T)(sg.elem)
					p.sendx = (p.sendx + 1) % p.dataqsiz
					p.qcount++
					sg.elem = nil
					sg.c = nil
					sg.success = true
					goready(sg.g, 0)
				}
			}
			unlock(&p.lock)
			return nil
		}
		if p.qcount <= s {
			var newBuf unsafe.Pointer
			if s > 0 {
				b := make([]T, s)
				if p.qcount > 0 && old > 0 {
					oldB := unsafe.Slice((*T)(p.buf), old)
					for i := uint(0); i < p.qcount; i++ {
						b[i] = oldB[(p.recvx+i)%old]
					}
				}
				newBuf = unsafe.Pointer(unsafe.SliceData(b))
			}
			p.buf = newBuf
			p.dataqsiz = s
			p.recvx = 0
			if s > 0 {
				p.sendx = p.qcount % s
			} else {
				p.sendx = 0
			}
			unlock(&p.lock)
			return nil
		}
		unlock(&p.lock)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			runtime.Gosched()
			time.Sleep(sleep)
			if sleep < 5*time.Millisecond {
				sleep *= 2
			}
		}
	}
}
