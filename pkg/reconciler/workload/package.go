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

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	workloadv1alpha1 "github.com/crossplane/crossplane/apis/workload/v1alpha1"
)

const (
	errWrapInKubeApp = "unable to wrap object in KubernetesApplication"
)

// A Packer is responsible for packaging workloads into other objects.
type Packer interface {
	Package(context.Context, Workload) (runtime.Object, error)
}

// A Packager is a concrete implementation of a Packer.
type Packager struct {
	PackageFn
}

// Package a workload into an object.
func (p *Packager) Package(ctx context.Context, w Workload) (runtime.Object, error) {
	return p.PackageFn(ctx, w)
}

// NewPackagerWithWrappers returns a Packer that packages and wraps a workload.
func NewPackagerWithWrappers(p PackageFn, wp ...PackageWrapper) Packer {
	return &Packager{
		PackageFn: func(ctx context.Context, w Workload) (runtime.Object, error) {
			obj, err := p(ctx, w)
			if err != nil {
				return nil, err
			}
			for _, wrap := range wp {
				if obj, err = wrap(ctx, w, obj); err != nil {
					return nil, err
				}
			}
			return obj, nil
		},
	}
}

// A PackageFn packages a workload into an object.
type PackageFn func(context.Context, Workload) (runtime.Object, error)

// Package workload into object with no wrappers.
func (fn PackageFn) Package(ctx context.Context, w Workload) (runtime.Object, error) {
	return fn(ctx, w)
}

var _ Packer = PackageFn(NoopPackage)

// NoopPackage does not package the workload and does not return error.
func NoopPackage(ctx context.Context, w Workload) (runtime.Object, error) {
	return nil, nil
}

// A PackageWrapper wraps the output of a workload package in another object.
type PackageWrapper func(context.Context, Workload, runtime.Object) (runtime.Object, error)

var _ PackageWrapper = NoopWrapper

// NoopWrapper does not wrap the workload package and does not return error.
func NoopWrapper(ctx context.Context, w Workload, obj runtime.Object) (runtime.Object, error) {
	return obj, nil
}

var _ PackageWrapper = KubeAppWrapper

// KubeAppWrapper wraps a packaged object in a KubernetesApplication.
func KubeAppWrapper(ctx context.Context, w Workload, obj runtime.Object) (runtime.Object, error) {
	if obj == nil {
		return nil, errors.New(errWrapInKubeApp)
	}
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	app := &workloadv1alpha1.KubernetesApplication{}

	kart := &workloadv1alpha1.KubernetesApplicationResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: w.GetName(),
			Labels: map[string]string{
				labelKey: string(w.GetUID()),
			},
		},
		Spec: workloadv1alpha1.KubernetesApplicationResourceSpec{
			Template: &unstructured.Unstructured{Object: u},
		},
	}

	app.Spec.ResourceTemplates = append(app.Spec.ResourceTemplates, *kart)
	// A workload's package must have the same name and namespace, and must be
	// controlled by the same owner.
	meta.AddOwnerReference(app, *metav1.NewControllerRef(w, w.GetObjectKind().GroupVersionKind()))
	app.SetName(w.GetName())
	app.SetNamespace(w.GetNamespace())

	app.Spec.ResourceSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			labelKey: string(w.GetUID()),
		},
	}

	return app, nil
}
