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

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/datastore"
	logutil "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/util/logging"
)

type NodeReconciler struct {
	client.Reader
	Datastore datastore.Datastore
}

func (c *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(logutil.VERBOSE).Info("Node being reconciled")

	node := &corev1.Node{}
	if err := c.Get(ctx, req.NamespacedName, node); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logutil.DEFAULT).Info("Node deleted from topology cache")
			c.Datastore.NodeDelete(req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("unable to get node - %w", err)
	}

	logger.V(logutil.DEFAULT).Info("Node topology cached", "node", node.Name)
	c.Datastore.NodeUpdateOrAddIfNotExist(node)
	return ctrl.Result{}, nil
}

func (c *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Watch all nodes - no filtering needed since we cache all node topology
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Complete(c)
}
