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

package datalayer

import "fmt"

// NodeMetadata represents the relevant Kubernetes Node state.
type NodeMetadata struct {
	NodeName string
	Labels   map[string]string
}

// String returns a string representation of the node metadata.
func (n *NodeMetadata) String() string {
	if n == nil {
		return ""
	}
	return fmt.Sprintf("NodeName: %s, Labels: %v", n.NodeName, n.Labels)
}

// Clone returns a full copy of the object.
func (n *NodeMetadata) Clone() *NodeMetadata {
	if n == nil {
		return nil
	}

	clonedLabels := make(map[string]string, len(n.Labels))
	for key, value := range n.Labels {
		clonedLabels[key] = value
	}
	return &NodeMetadata{
		NodeName: n.NodeName,
		Labels:   clonedLabels,
	}
}
