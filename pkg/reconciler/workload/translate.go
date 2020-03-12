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
	"reflect"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	workloadv1alpha1 "github.com/crossplane/crossplane/apis/workload/v1alpha1"
)

const (
	errWrapInKubeApp = "unable to wrap objects in KubernetesApplication"
	errInjectService = "unable to inject Service in objects"
)

var (
	deploymentGroupVersionKind = appsv1.SchemeGroupVersion.WithKind(reflect.TypeOf(appsv1.Deployment{}).Name())

	serviceKind       = reflect.TypeOf(corev1.Service{}).Name()
	serviceAPIVersion = corev1.SchemeGroupVersion.String()
)

// A Translator is responsible for packaging workloads into other objects.
type Translator interface {
	Translate(context.Context, Workload) ([]Object, error)
}

// An ObjectTranslator is a concrete implementation of a Translator.
type ObjectTranslator struct {
	TranslateFn
}

// Translate a workload into other objects.
func (p *ObjectTranslator) Translate(ctx context.Context, w Workload) ([]Object, error) {
	return p.TranslateFn(ctx, w)
}

// NewObjectTranslatorWithWrappers returns a Translator that translates and wraps
// a workload.
func NewObjectTranslatorWithWrappers(t TranslateFn, wp ...TranslationWrapper) Translator {
	return &ObjectTranslator{
		TranslateFn: func(ctx context.Context, w Workload) ([]Object, error) {
			objs, err := t(ctx, w)
			if err != nil {
				return nil, err
			}
			for _, wrap := range wp {
				if objs, err = wrap(ctx, w, objs); err != nil {
					return nil, err
				}
			}
			return objs, nil
		},
	}
}

// A TranslateFn translates a workload into an object.
type TranslateFn func(context.Context, Workload) ([]Object, error)

// Translate workload into object or objects with no wrappers.
func (fn TranslateFn) Translate(ctx context.Context, w Workload) ([]Object, error) {
	return fn(ctx, w)
}

var _ Translator = TranslateFn(NoopTranslate)

// NoopTranslate does not translate the workload and does not return error.
func NoopTranslate(ctx context.Context, w Workload) ([]Object, error) {
	return nil, nil
}

// A TranslationWrapper wraps the output of a workload translation in another
// object or adds addition object.
type TranslationWrapper func(context.Context, Workload, []Object) ([]Object, error)

var _ TranslationWrapper = NoopWrapper

// NoopWrapper does not wrap the workload translation and does not return error.
func NoopWrapper(ctx context.Context, w Workload, objs []Object) ([]Object, error) {
	return objs, nil
}

var _ TranslationWrapper = KubeAppWrapper

// KubeAppWrapper wraps a set of translated objects in a KubernetesApplication.
func KubeAppWrapper(ctx context.Context, w Workload, objs []Object) ([]Object, error) {
	if objs == nil {
		return nil, nil
	}

	app := &workloadv1alpha1.KubernetesApplication{}

	for _, o := range objs {
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(o)
		if err != nil {
			return nil, errors.Wrap(err, errWrapInKubeApp)
		}

		kart := workloadv1alpha1.KubernetesApplicationResourceTemplate{
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

		app.Spec.ResourceTemplates = append(app.Spec.ResourceTemplates, kart)
	}

	// A workload's package must have the same name and namespace, and must be
	// controlled by the same owner.
	app.SetName(w.GetName())
	app.SetNamespace(w.GetNamespace())

	app.Spec.ResourceSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			labelKey: string(w.GetUID()),
		},
	}

	return []Object{app}, nil
}

var _ TranslationWrapper = ServiceInjector

// ServiceInjector adds a Service object for every Deployment that has a
// container with port defined into a workload translation.
func ServiceInjector(ctx context.Context, w Workload, objs []Object) ([]Object, error) {
	if objs == nil {
		return nil, nil
	}

	for _, o := range objs {
		if o.GetObjectKind().GroupVersionKind() != deploymentGroupVersionKind {
			continue
		}

		d, ok := o.(*appsv1.Deployment)
		if !ok {
			return nil, errors.New(errInjectService)
		}

		for _, c := range d.Spec.Template.Spec.Containers {
			serviceAdded := false
			for _, port := range c.Ports {
				s := &corev1.Service{
					TypeMeta: metav1.TypeMeta{
						Kind:       serviceKind,
						APIVersion: serviceAPIVersion,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: d.GetName(),
						Labels: map[string]string{
							labelKey: string(w.GetUID()),
						},
					},
					Spec: corev1.ServiceSpec{
						Selector: d.Spec.Selector.MatchLabels,
						Ports: []corev1.ServicePort{
							{
								Name:       d.GetName(),
								Port:       8080,
								TargetPort: intstr.FromInt(int(port.ContainerPort)),
							},
						},
						Type: corev1.ServiceTypeLoadBalancer,
					},
				}
				objs = append(objs, s)
				break
			}
			if serviceAdded {
				break
			}
		}
	}
	return objs, nil
}
