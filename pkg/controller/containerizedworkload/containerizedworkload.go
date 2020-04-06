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
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/oam/workload"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	oamv1alpha2 "github.com/crossplane/crossplane/apis/oam/v1alpha2"
	client "github.com/crossplane/crossplane/pkg/oam/workload"
	"github.com/crossplane/crossplane/pkg/oam/workload/containerized"
	apply "github.com/crossplane/crossplane/pkg/workload"
)

// SetupContainerizedWorkload adds a controller that reconciles ContainerizedWorkloads.
func SetupContainerizedWorkload(mgr ctrl.Manager, l logging.Logger) error {
	name := "oam/" + strings.ToLower(oamv1alpha2.ContainerizedWorkloadGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&oamv1alpha2.ContainerizedWorkload{}).
		Complete(workload.NewReconciler(mgr,
			resource.WorkloadKind(oamv1alpha2.ContainerizedWorkloadGroupVersionKind),
			workload.WithLogger(l.WithValues("controller", name)),
			workload.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
			workload.WithApplyOptions(resource.ControllersMustMatch(), apply.KubeAppApplyOption()),
			workload.WithTranslator(workload.NewObjectTranslatorWithWrappers(
				containerized.Translator,
				client.ServiceInjector,
				client.KubeAppWrapper,
			)),
		))
}
