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
	"time"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	oamv1alpha2 "github.com/crossplane/crossplane/apis/oam/v1alpha2"
)

const (
	reconcileTimeout = 1 * time.Minute
	shortWait        = 30 * time.Second
	longWait         = 1 * time.Minute
)

// Reconcile error strings.
const (
	errGetTrait               = "cannot get trait"
	errUpdateTraitStatus      = "cannot update trait status"
	errTraitModify            = "cannot apply trait modification"
	errGetPackage             = "cannot get package for workload reference in trait"
	errApplyTraitModification = "cannot apply trait modification to workload package"
)

// Reconcile event reasons.
const (
	reasonTraitWait   = "WaitingForWorkloadPackage"
	reasonTraitModify = "PackageModified"

	reasonCannotGetPackage        = "CannotGetReferencedWorkloadPackage"
	reasonCannotModifyPackage     = "CannotModifyPackage"
	reasonCannotApplyModification = "CannotApplyModification"
)

// A ReconcilerOption configures a Reconciler.
type ReconcilerOption func(*Reconciler)

// WithLogger specifies how the Reconciler should log messages.
func WithLogger(l logging.Logger) ReconcilerOption {
	return func(r *Reconciler) {
		r.log = l
	}
}

// WithRecorder specifies how the Reconciler should record events.
func WithRecorder(er event.Recorder) ReconcilerOption {
	return func(r *Reconciler) {
		r.record = er
	}
}

// WithModifier specifies how the Reconciler should modify the workload package.
func WithModifier(m Modifier) ReconcilerOption {
	return func(r *Reconciler) {
		r.trait = m
	}
}

// WithAccessors specifies how the Reconciler should access the part of the
// workload package to be modified.
func WithAccessors(m ModifyAccessor) ReconcilerOption {
	return func(r *Reconciler) {
		r.accessor = m
	}
}

// WithApplicator specifies how the Reconciler should apply the workload
// package modification.
func WithApplicator(a resource.Applicator) ReconcilerOption {
	return func(r *Reconciler) {
		r.applicator = a
	}
}

// A Reconciler reconciles OAM traits by modifying the object that a workload
// has been packaged into.
type Reconciler struct {
	client     client.Client
	newTrait   func() Trait
	newPackage func() Object
	trait      Modifier
	accessor   ModifyAccessor
	applicator resource.Applicator

	log    logging.Logger
	record event.Recorder
}

// Kind is an OAM trait kind.
type Kind schema.GroupVersionKind

// A WorkloadReferencer is a type that has a Workload reference.
type WorkloadReferencer interface {
	GetWorkloadReference() oamv1alpha2.WorkloadReference
	SetWorkloadReference(oamv1alpha2.WorkloadReference)
}

// A Trait is a type of OAM trait.
type Trait interface {
	resource.Conditioned
	metav1.Object
	runtime.Object
	WorkloadReferencer
}

// Object is a Kubernetes object.
type Object interface {
	metav1.Object
	runtime.Object
}

// NewReconciler returns a Reconciler that reconciles OAM traits by fetching
// their referenced workload's package and applying modifications.
func NewReconciler(m ctrl.Manager, trait Kind, p Kind, o ...ReconcilerOption) *Reconciler {
	nt := func() Trait {
		return resource.MustCreateObject(schema.GroupVersionKind(trait), m.GetScheme()).(Trait)
	}

	np := func() Object {
		return resource.MustCreateObject(schema.GroupVersionKind(p), m.GetScheme()).(Object)
	}

	r := &Reconciler{
		client:     m.GetClient(),
		newTrait:   nt,
		newPackage: np,
		trait:      ModifyFn(NoopModifier),
		accessor:   NoopModifyAccessor,
		applicator: resource.ApplyFn(resource.Apply),

		log:    logging.NewNopLogger(),
		record: event.NewNopRecorder(),
	}

	for _, ro := range o {
		ro(r)
	}

	return r
}

// Reconcile an OAM trait type by modifying its referenced workload's
// KubernetesApplication.
func (r *Reconciler) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("request", req)
	log.Debug("Reconciling")

	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()

	trait := r.newTrait()
	if err := r.client.Get(ctx, req.NamespacedName, trait); err != nil {
		return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), errGetTrait)
	}

	log = log.WithValues("uid", trait.GetUID(), "version", trait.GetResourceVersion())

	pack := r.newPackage()
	err := r.client.Get(ctx, types.NamespacedName{Name: trait.GetWorkloadReference().Name, Namespace: trait.GetNamespace()}, pack)
	if kerrors.IsNotFound(err) {
		log.Debug("Waiting for referenced workload's package", "kind", trait.GetObjectKind().GroupVersionKind().String())
		r.record.Event(trait, event.Normal(reasonTraitWait, "Waiting for workload package to exist"))
		trait.SetConditions(v1alpha1.ReconcileSuccess())
		return reconcile.Result{RequeueAfter: shortWait}, errors.Wrap(r.client.Status().Update(ctx, trait), errUpdateTraitStatus)
	}
	if err != nil {
		log.Debug("Cannot get workload package", "error", err, "requeue-after", time.Now().Add(shortWait))
		r.record.Event(trait, event.Warning(reasonCannotGetPackage, err))
		trait.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errGetPackage)))
		return reconcile.Result{RequeueAfter: shortWait}, errors.Wrap(r.client.Status().Update(ctx, trait), errUpdateTraitStatus)
	}

	if err := r.trait.Modify(ctx, pack, trait, r.accessor); err != nil {
		log.Debug("Cannot modify workload package", "error", err, "requeue-after", time.Now().Add(shortWait))
		r.record.Event(trait, event.Warning(reasonCannotModifyPackage, err))
		trait.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errTraitModify)))
		return reconcile.Result{RequeueAfter: shortWait}, errors.Wrap(r.client.Status().Update(ctx, trait), errUpdateTraitStatus)
	}

	// The trait's referenced workload should always be packaged in a
	// KubernetesApplication that is controlled by the same owner. In the case
	// where a KubernetesApplication already exists in the same namespace and
	// with the same name as the workload before it is created, this wll guard
	// against modifying it.
	if err := r.applicator.Apply(ctx, r.client, pack, resource.ControllersMustMatch()); err != nil {
		log.Debug("Cannot apply workload package", "error", err, "requeue-after", time.Now().Add(shortWait))
		r.record.Event(trait, event.Warning(reasonCannotApplyModification, err))
		trait.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errApplyTraitModification)))
		return reconcile.Result{RequeueAfter: shortWait}, errors.Wrap(r.client.Status().Update(ctx, trait), errUpdateTraitStatus)
	}

	r.record.Event(trait, event.Normal(reasonTraitModify, "Successfully modifed workload package"))
	log.Debug("Successfully modified referenced workload's KubernetesApplication", "kind", trait.GetObjectKind().GroupVersionKind().String())

	// trait.SetConditions(v1alpha1.ReconcileSuccess())
	return reconcile.Result{RequeueAfter: longWait}, errors.Wrap(r.client.Status().Update(ctx, trait), errUpdateTraitStatus)
}
