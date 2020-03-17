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

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	workloadv1alpha1 "github.com/crossplane/crossplane/apis/workload/v1alpha1"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	errNotKubeApp            = "object is not a KubernetesApplication"
	errCreateKubeApp         = "cannot create KubernetesApplication"
	errGetKubeApp            = "cannot get KubernetesApplication"
	errDiffController        = "existing KubernetesApplication has a different (or no) controller"
	errMergeKubeAppTemplates = "cannot merge KubernetesApplicationResourceTemplates"
	errPatchKubeApp          = "cannot patch existing KubernetesApplication"
)

// KubeAppApplyOption ensures resource templates are merged instead of replaced
// before patch if they have the same name and GroupVersionKind.
func KubeAppApplyOption() resource.ApplyOption {
	return func(_ context.Context, current, desired runtime.Object) error {
		c, ok := current.(*workloadv1alpha1.KubernetesApplication)
		if !ok {
			return errors.New(errNotKubeApp)
		}
		d, ok := desired.(*workloadv1alpha1.KubernetesApplication)
		if !ok {
			return errors.New(errNotKubeApp)
		}
		for i, temp := range c.Spec.ResourceTemplates {
			if d.Spec.ResourceTemplates[i].Spec.Template.GroupVersionKind() != temp.Spec.Template.GroupVersionKind() {
				continue
			}
			if d.Spec.ResourceTemplates[i].GetName() != temp.GetName() {
				continue
			}
			jc, err := json.Marshal(temp.Spec.Template)
			if err != nil {
				return errors.Wrap(err, errMergeKubeAppTemplates)
			}
			jd, err := json.Marshal(d.Spec.ResourceTemplates[i].Spec.Template)
			if err != nil {
				return errors.Wrap(err, errMergeKubeAppTemplates)
			}
			merge, err := jsonpatch.MergePatch(jc, jd)
			if err != nil {
				return errors.Wrap(err, errMergeKubeAppTemplates)
			}
			if err := json.Unmarshal(merge, d.Spec.ResourceTemplates[i].Spec.Template); err != nil {
				return errors.Wrap(err, errMergeKubeAppTemplates)
			}
		}
		return nil
	}
}
