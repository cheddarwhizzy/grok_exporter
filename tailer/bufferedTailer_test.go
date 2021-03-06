// Copyright 2016-2018 The grok_exporter Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tailer

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

type sourceTailer struct {
	lines chan string
}

func (tail *sourceTailer) Lines() chan string {
	return tail.lines
}

func (tail *sourceTailer) Errors() chan Error {
	return nil
}

func (tail *sourceTailer) Close() {
	close(tail.lines)
}

// First produce 10,000 lines, then consume 10,000 lines.
func TestLineBufferSequential(t *testing.T) {
	src := &sourceTailer{lines: make(chan string)}
	metric := &peakLoadMetric{}
	buffered := BufferedTailerWithMetrics(src, metric)
	for i := 1; i <= 10000; i++ {
		src.lines <- fmt.Sprintf("This is line number %v.", i)
	}
	for i := 1; i <= 10000; i++ {
		line := <-buffered.Lines()
		if line != fmt.Sprintf("This is line number %v.", i) {
			t.Errorf("Expected 'This is line number %v', but got '%v'.", i, line)
		}
	}
	// wait until peak load is observed (buffered tailer observes the max of each 1 Sec interval)
	time.Sleep(1100 * time.Millisecond)
	buffered.Close()
	_, stillOpen := <-buffered.Lines()
	if stillOpen {
		t.Error("Buffered tailer was not closed.")
	}
	_, stillOpen = <-src.Lines()
	if stillOpen {
		t.Error("Source tailer was not closed.")
	}
	if !metric.registerCalled {
		t.Error("metric.Register() not called.")
	}
	if !metric.unregisterCalled {
		t.Error("metric.Unregister() not called.")
	}
	// The peak load should be 9999 or 9998, depending on how quick
	// the consumer loop started reading
	fmt.Printf("peak load: %v\n", metric.peakLoad)
}

// Produce and consume in parallel.
func TestLineBufferParallel(t *testing.T) {
	src := &sourceTailer{lines: make(chan string)}
	metric := &peakLoadMetric{}
	buffered := BufferedTailerWithMetrics(src, metric)
	var wg sync.WaitGroup
	go func() {
		start := time.Now()
		for i := 1; i <= 10000; i++ {
			src.lines <- fmt.Sprintf("This is line number %v.", i)
			if rand.Int()%64 == 0 { // Sleep from time to time
				time.Sleep(10 * time.Millisecond)
			}
		}
		fmt.Printf("Producer took %v.\n", time.Since(start))
		wg.Done()
	}()
	go func() {
		start := time.Now()
		for i := 1; i <= 10000; i++ {
			line := <-buffered.Lines()
			if line != fmt.Sprintf("This is line number %v.", i) {
				t.Errorf("Expected 'This is line number %v', but got '%v'.", i, line)
			}
			if rand.Int()%64 == 0 { // Sleep from time to time
				time.Sleep(10 * time.Millisecond)
			}
		}
		fmt.Printf("Consumer took %v.\n", time.Since(start))
		wg.Done()
	}()
	wg.Add(2)
	wg.Wait()
	// wait until peak load is observed (buffered tailer observes the max of each 1 Sec interval)
	time.Sleep(1100 * time.Millisecond)
	buffered.Close()
	_, stillOpen := <-buffered.Lines()
	if stillOpen {
		t.Error("Buffered tailer was not closed.")
	}
	_, stillOpen = <-src.Lines()
	if stillOpen {
		t.Error("Source tailer was not closed.")
	}
	if !metric.registerCalled {
		t.Error("metric.Register() not called.")
	}
	if !metric.unregisterCalled {
		t.Error("metric.Unregister() not called.")
	}
	// Should be much less than 10000, because consumer and producer work in parallel.
	fmt.Printf("peak load: %v\n", metric.peakLoad)
}

type peakLoadMetric struct {
	registerCalled, unregisterCalled bool
	peakLoad                         float64
}

func (m *peakLoadMetric) Register() {
	m.registerCalled = true
}

func (m *peakLoadMetric) Observe(currentLoad float64) {
	if currentLoad > m.peakLoad {
		m.peakLoad = currentLoad
	}
}

func (m *peakLoadMetric) Unregister() {
	m.unregisterCalled = true
}
