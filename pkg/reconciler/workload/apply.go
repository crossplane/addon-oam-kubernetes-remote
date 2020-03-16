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

package workload

import (
	"context"
	"encoding/json"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	workloadv1alpha1 "github.com/crossplane/crossplane/apis/workload/v1alpha1"
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errNotKubeApp            = "object is not a KubernetesApplication"
	errCreateKubeApp         = "cannot create KubernetesApplication"
	errGetKubeApp            = "cannot get KubernetesApplication"
	errDiffController        = "existing KubernetesApplication has a different (or no) controller"
	errMergeKubeAppTemplates = "cannot merge KubernetesApplicationResourceTemplates"
	errPatchKubeApp          = "cannot patch existing KubernetesApplication"
)

// KubeAppApply applies changes to the supplied KubernetesApplication. The
// KubernetesApplication will be created if it does not exist, or patched if it
// does. Resource templates are merged instead of replaced if they have the same
// name and GroupVersionKind.
func KubeAppApply(ctx context.Context, c client.Client, o runtime.Object, ao ...resource.ApplyOption) error {
	opts := &resource.ApplyOptions{}
	for _, fn := range ao {
		fn(opts)
	}

	m, ok := o.(*workloadv1alpha1.KubernetesApplication)
	if !ok {
		return errors.New(errNotKubeApp)
	}

	desired := &workloadv1alpha1.KubernetesApplication{}
	m.DeepCopyInto(desired)

	err := c.Get(ctx, types.NamespacedName{Name: m.GetName(), Namespace: m.GetNamespace()}, o)
	if kerrors.IsNotFound(err) {
		return errors.Wrap(c.Create(ctx, o), errCreateKubeApp)
	}
	if err != nil {
		return errors.Wrap(err, errGetKubeApp)
	}

	if opts.ControllersMustMatch && !meta.HaveSameController(m, desired) {
		return errors.New(errDiffController)
	}

	for i, temp := range m.Spec.ResourceTemplates {
		if desired.Spec.ResourceTemplates[i].Spec.Template.GroupVersionKind() != temp.Spec.Template.GroupVersionKind() {
			continue
		}
		if desired.Spec.ResourceTemplates[i].GetName() != temp.GetName() {
			continue
		}
		if err := mergo.Merge(desired.Spec.ResourceTemplates[i].Spec.Template, temp.Spec.Template); err != nil {
			return errors.Wrap(err, errMergeKubeAppTemplates)
		}
	}

	return errors.Wrap(c.Patch(ctx, o, &patch{desired}), errPatchKubeApp)
}

type patch struct{ from runtime.Object }

func (p *patch) Type() types.PatchType                 { return types.MergePatchType }
func (p *patch) Data(_ runtime.Object) ([]byte, error) { return json.Marshal(p.from) }
