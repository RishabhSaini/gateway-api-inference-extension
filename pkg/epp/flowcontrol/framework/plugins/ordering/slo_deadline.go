/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ordering

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/gateway-api-inference-extension/pkg/common/request"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/framework/interface/flowcontrol"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/framework/interface/plugin"
)

const (
	// SLODeadlineOrderingPolicyType orders requests by an SLO-based deadline
	//
	// It selects the request with the earliest SLO-based deadline, computed as `ReceivedTimestamp() + x-slo-ttft-ms header (interpreted as milliseconds)`.
	// Requests without a valid x-slo-ttft-ms header are treated as having no deadline and are scheduled after SLO-bound requests,
	// with FCFS as a tie-breaker.
	SLODeadlineOrderingPolicyType = "slo-deadline-ordering-policy"

	// sloTtftHeader is the request header name for SLO time-to-first-token in milliseconds.
	sloTtftHeader = "x-slo-ttft-ms"
)

func SLODeadlineOrderingPolicyFactory(name string, _ json.RawMessage, _ plugin.Handle) (plugin.Plugin, error) {
	return newSLODeadlinePolicy().withName(name), nil
}

type sloDeadlinePolicy struct {
	name string
}

var _ flowcontrol.OrderingPolicy = &sloDeadlinePolicy{}

func newSLODeadlinePolicy() *sloDeadlinePolicy {
	return &sloDeadlinePolicy{
		name: SLODeadlineOrderingPolicyType,
	}
}

func (p *sloDeadlinePolicy) withName(name string) *sloDeadlinePolicy {
	if name != "" {
		p.name = name
	}
	return p
}

func (p *sloDeadlinePolicy) Name() string {
	return p.name
}

// RequiredQueueCapabilities returns the queue capabilities required by this policy.
func (p *sloDeadlinePolicy) RequiredQueueCapabilities() []flowcontrol.QueueCapability {
	return []flowcontrol.QueueCapability{flowcontrol.CapabilityPriorityConfigurable}
}

func (p *sloDeadlinePolicy) TypedName() plugin.TypedName {
	return plugin.TypedName{
		Type: SLODeadlineOrderingPolicyType,
		Name: p.name,
	}
}

var sloMaxDeadlineTime = time.Unix(0, 1<<63-1)

// calculateSLODeadline computes the SLO-based deadline for a request: ReceivedTimestamp + x-slo-ttft-ms (ms).
// The header is read from the InferenceRequest()'s headers. If the header is missing, empty, or invalid,
// the request is assigned a far-future deadline so it sorts after SLO-bound requests.
var sloDebugCounter int64

func calculateSLODeadline(item flowcontrol.QueueItemAccessor) time.Time {
	req := item.OriginalRequest()
	if req == nil {
		fmt.Println("[DEBUG-SLO] calculateSLODeadline: req is nil, returning max deadline")
		return sloMaxDeadlineTime
	}
	infReq := req.InferenceRequest()
	if infReq == nil || infReq.Headers == nil {
		fmt.Printf("[DEBUG-SLO] calculateSLODeadline: infReq=%v headers=%v, returning max deadline\n", infReq != nil, infReq != nil && infReq.Headers != nil)
		return sloMaxDeadlineTime
	}
	sloTtft := request.GetHeader(infReq.Headers, sloTtftHeader)
	if sloTtft == "" {
		// Log first 5 occurrences + every 50th to avoid spam
		sloDebugCounter++
		if sloDebugCounter <= 5 || sloDebugCounter%50 == 0 {
			fmt.Printf("[DEBUG-SLO] calculateSLODeadline: x-slo-ttft-ms header NOT FOUND (count=%d), available headers: %v\n", sloDebugCounter, infReq.Headers)
		}
		return sloMaxDeadlineTime
	}
	ms, err := strconv.ParseInt(strings.TrimSpace(sloTtft), 10, 64)
	if err != nil || ms < 0 {
		fmt.Printf("[DEBUG-SLO] calculateSLODeadline: invalid x-slo-ttft-ms value=%q err=%v\n", sloTtft, err)
		return sloMaxDeadlineTime
	}
	deadline := req.ReceivedTimestamp().Add(time.Duration(ms) * time.Millisecond)
	fmt.Printf("[DEBUG-SLO] calculateSLODeadline: slo=%dms received=%v deadline=%v\n", ms, req.ReceivedTimestamp().Format(time.RFC3339Nano), deadline.Format(time.RFC3339Nano))
	return deadline
}

var lessCallCounter int64

// Less returns true if item 'a' should be dispatched before item 'b'.
// It orders by SLO deadline (earliest first), using FCFS as a tie-breaker.
func (p *sloDeadlinePolicy) Less(a, b flowcontrol.QueueItemAccessor) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	deadlineA := calculateSLODeadline(a)
	deadlineB := calculateSLODeadline(b)

	lessCallCounter++
	if lessCallCounter <= 10 || lessCallCounter%100 == 0 {
		isMax := deadlineA.Equal(sloMaxDeadlineTime) || deadlineB.Equal(sloMaxDeadlineTime)
		fmt.Printf("[DEBUG-SLO] Less(count=%d): deadlineA=%v deadlineB=%v aBeforeB=%v hasMaxDeadline=%v\n",
			lessCallCounter, deadlineA.Format(time.RFC3339Nano), deadlineB.Format(time.RFC3339Nano),
			deadlineA.Before(deadlineB), isMax)
	}

	if !deadlineA.Equal(deadlineB) {
		return deadlineA.Before(deadlineB)
	}
	reqA := a.OriginalRequest()
	reqB := b.OriginalRequest()
	if reqA == nil && reqB == nil {
		return false
	}
	if reqA == nil {
		return false
	}
	if reqB == nil {
		return true
	}
	return reqA.ReceivedTimestamp().Before(reqB.ReceivedTimestamp())
}
