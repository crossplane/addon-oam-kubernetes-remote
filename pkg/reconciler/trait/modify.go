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

package trait

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	workloadv1alpha1 "github.com/crossplane/crossplane/apis/workload/v1alpha1"
)

const (
	errNotKubeApp           = "object passed to KubernetesApplication accessor is not KubernetesApplication"
	errNoDeploymentForTrait = "no deployment found for trait in KubernetesApplication"
)

var (
	deploymentKind = reflect.TypeOf(appsv1.Deployment{}).Name()
)

// A Modifier is responsible for modifying or adding objects to a workload
// translation.
type Modifier interface {
	Modify(context.Context, runtime.Object, Trait) error
}

// WorkloadModifier is a concrete implementation of a Modifier.
type WorkloadModifier struct {
	ModifyFn
}

// Modify modifies or adds an object in a workload translation.
func (m *WorkloadModifier) Modify(ctx context.Context, obj runtime.Object, t Trait) error {
	return m.ModifyFn(ctx, obj, t)
}

// NewWorkloadModifierWithAccessor is a modifier of a workload translation that uses an accessor.
func NewWorkloadModifierWithAccessor(m ModifyFn, a ModifyAccessor) Modifier {
	return &WorkloadModifier{
		ModifyFn: func(ctx context.Context, obj runtime.Object, t Trait) error { return a(ctx, obj, t, m) },
	}
}

// A ModifyFn modifies or adds an object to a workload translation.
type ModifyFn func(ctx context.Context, obj runtime.Object, t Trait) error

// Modify object in workload translation.
func (fn ModifyFn) Modify(ctx context.Context, obj runtime.Object, t Trait) error {
	return fn(ctx, obj, t)
}

var _ Modifier = ModifyFn(NoopModifier)

// NoopModifier makes no modifications and returns no errors.
func NoopModifier(_ context.Context, _ runtime.Object, _ Trait) error {
	return nil
}

// A ModifyAccessor obtains the object to be modified from a wrapping object.
type ModifyAccessor func(context.Context, runtime.Object, Trait, ModifyFn) error

var _ ModifyAccessor = NoopModifyAccessor

// NoopModifyAccessor passes the provided object to the modifier as-is.
func NoopModifyAccessor(ctx context.Context, obj runtime.Object, t Trait, m ModifyFn) error {
	return m(ctx, obj, t)
}

var _ ModifyAccessor = DeploymentFromKubeAppAccessor

// DeploymentFromKubeAppAccessor gets deployments from a KubernetesApplication.
func DeploymentFromKubeAppAccessor(ctx context.Context, obj runtime.Object, t Trait, m ModifyFn) error {
	a, ok := obj.(*workloadv1alpha1.KubernetesApplication)
	if !ok {
		return errors.New(errNotKubeApp)
	}

	for i, r := range a.Spec.ResourceTemplates {
		template := &unstructured.Unstructured{}
		if err := json.Unmarshal(r.Spec.Template.Raw, template); err != nil {
			return err
		}
		if template.GroupVersionKind().Kind == deploymentKind {
			d := &appsv1.Deployment{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.UnstructuredContent(), d); err != nil {
				return err
			}
			if err := m(ctx, d, t); err != nil {
				return err
			}
			deployment, err := json.Marshal(d)
			if err != nil {
				return err
			}
			a.Spec.ResourceTemplates[i].Spec.Template = runtime.RawExtension{Raw: deployment}
			return nil
		}
	}

	return errors.New(errNoDeploymentForTrait)
}
