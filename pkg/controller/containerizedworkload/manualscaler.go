/*
Copyright 2020 The Crossplane Authors.

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

package containerizedworkload

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	oamv1alpha2 "github.com/crossplane/crossplane/apis/oam/v1alpha2"
	workloadv1alpha1 "github.com/crossplane/crossplane/apis/workload/v1alpha1"

	"github.com/crossplane/addon-oam-kubernetes-remote/pkg/reconciler/trait"
)

const (
	errNotDeployment        = "object to be modified is not a deployment"
	errNotManualScalerTrait = "trait is not a manual scaler"
)

// SetupManualScalerTrait adds a controller that reconciles ManualScalers that
// reference a ContainerizedWorkload.
func SetupManualScalerTrait(mgr ctrl.Manager, l logging.Logger) error {
	name := "oam/" + strings.ToLower(oamv1alpha2.ManualScalerTraitGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&oamv1alpha2.ManualScalerTrait{}).
		Complete(trait.NewReconciler(mgr,
			trait.Kind(oamv1alpha2.ManualScalerTraitGroupVersionKind),
			trait.Kind(workloadv1alpha1.KubernetesApplicationGroupVersionKind),
			trait.WithLogger(l.WithValues("controller", name)),
			trait.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
			trait.WithModifier(trait.NewWorkloadModifierWithAccessor(manualScalerModifier, trait.DeploymentFromKubeAppAccessor)),
		))
}

func manualScalerModifier(ctx context.Context, obj runtime.Object, t trait.Trait) error {
	d, ok := obj.(*appsv1.Deployment)
	if !ok {
		return errors.New(errNotDeployment)
	}

	ms, ok := t.(*oamv1alpha2.ManualScalerTrait)
	if !ok {
		return errors.New(errNotManualScalerTrait)
	}
	d.Spec.Replicas = &ms.Spec.ReplicaCount

	return nil
}
