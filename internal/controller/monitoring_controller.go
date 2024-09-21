/*
Copyright 2024.

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

	"github.com/go-logr/logr"
	"github.com/wnguddn777.com/autodeploy/internal/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	operatorv1alpha1 "github.com/wnguddn777.com/autodeploy/api/v1alpha1"
)

const monitoringFinalizer = "operator.wnguddn777.com/finalizer"

type MonitoringReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func split_point() int {
	return 2
}

func (r *MonitoringReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("monitoring", req.NamespacedName)

	var monitoring operatorv1alpha1.Monitoring
	if err := r.Get(ctx, req.NamespacedName, &monitoring); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 파이널라이저 처리
	if monitoring.ObjectMeta.DeletionTimestamp.IsZero() {
		if !utils.ContainsString(monitoring.GetFinalizers(), monitoringFinalizer) {
			controllerutil.AddFinalizer(&monitoring, monitoringFinalizer)
			if err := r.Update(ctx, &monitoring); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if utils.ContainsString(monitoring.GetFinalizers(), monitoringFinalizer) {
			if err := r.deleteExternalResources(ctx, &monitoring); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&monitoring, monitoringFinalizer)
			if err := r.Update(ctx, &monitoring); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	splitPoint := split_point()

	for _, edge := range monitoring.Spec.Edge {
		image := fmt.Sprintf("bono1130/head-model-%d", splitPoint)
		if err := r.createOrUpdateAutoDeploy(ctx, image, edge, &monitoring); err != nil {
			log.Error(err, "Failed to create or update Head AutoDeploy")
			return ctrl.Result{}, err
		}
	}

	image := fmt.Sprintf("bono1130/tail-model-%d", splitPoint)
	if err := r.createOrUpdateAutoDeploy(ctx, image, monitoring.Spec.Core, &monitoring); err != nil {
		log.Error(err, "Failed to create or update Tail AutoDeploy")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func CreateAutoDeploy(image, memberCluster string) *operatorv1alpha1.AutoDeploy {
	return &operatorv1alpha1.AutoDeploy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("autodeploy-%s", memberCluster),
			Namespace: "default",
		},
		Spec: operatorv1alpha1.AutoDeploySpec{
			Image:         image,
			MemberCluster: memberCluster,
			Replicas:      1, // 기본값으로 1을 설정, 필요에 따라 조정 가능
		},
	}
}

func (r *MonitoringReconciler) createOrUpdateAutoDeploy(ctx context.Context, image, memberCluster string, monitoring *operatorv1alpha1.Monitoring) error {
	autoDeploy := CreateAutoDeploy(image, memberCluster)
	if err := controllerutil.SetControllerReference(monitoring, autoDeploy, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, autoDeploy); err != nil {
		if errors.IsAlreadyExists(err) {
			existing := &operatorv1alpha1.AutoDeploy{}
			if err := r.Get(ctx, client.ObjectKeyFromObject(autoDeploy), existing); err != nil {
				return err
			}
			existing.Spec = autoDeploy.Spec
			return r.Update(ctx, existing)
		}
		return err
	}
	return nil
}

func (r *MonitoringReconciler) deleteExternalResources(ctx context.Context, monitoring *operatorv1alpha1.Monitoring) error {
	// AutoDeploy CR 삭제
	autoDeployList := &operatorv1alpha1.AutoDeployList{}
	if err := r.List(ctx, autoDeployList, client.InNamespace(monitoring.Namespace)); err != nil {
		return err
	}
	for _, autoDeploy := range autoDeployList.Items {
		if err := r.Delete(ctx, &autoDeploy); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// SetupWithManager 함수는 그대로 유지

// SetupWithManager sets up the controller with the Manager.
func (r *MonitoringReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.Monitoring{}).
		Complete(r)
}
