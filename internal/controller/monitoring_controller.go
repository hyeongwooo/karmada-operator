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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha1 "github.com/wnguddn777.com/autodeploy/api/v1alpha1"
	// "github.com/wnguddn777.com/autodeploy/internal/prom_op"
)

// MonitoringReconciler reconciles a Monitoring object
type MonitoringReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=operator.wnguddn777.com,resources=monitorings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.wnguddn777.com,resources=monitorings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.wnguddn777.com,resources=monitorings/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Monitoring object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile

// split_point 함수: Karmada의 Prometheus API를 사용하여 CPU 리소스를 확인하고 split point를 결정합니다.
func split_point() int {
	// cpuUsage, err := prom_op.GetClusterCPUUsage(cluster, prometheusURL) // prom_op 모듈 사용
	// if err != nil {
	// 	fmt.Printf("Failed to get CPU usage for cluster %s: %v\n", cluster, err)
	// 	return 1 // 기본 값
	// }

	// // CPU 사용량을 기준으로 split point 결정
	// if cpuUsage < 50 {
	// 	return 1
	// } else if cpuUsage < 70 {
	// 	return 2
	// }
	// return 3
	return 2
}

// CreateAutoDeploy 함수: AutoDeploy CR을 생성합니다.
func CreateAutoDeploy(image, memberCluster string) *operatorv1alpha1.AutoDeploy {
	return &operatorv1alpha1.AutoDeploy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("autodeploy-%s", memberCluster),
			Namespace: "default",
		},
		Spec: operatorv1alpha1.AutoDeploySpec{
			Image:         image,
			MemberCluster: memberCluster,
			Replicas:      1,
		},
	}
}

// Reconcile 함수: Monitoring CR의 변화를 감지하여 적절한 작업을 수행합니다.
func (r *MonitoringReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("monitoring", req.NamespacedName)

	// Monitoring CR 인스턴스 가져오기
	var monitoring operatorv1alpha1.Monitoring
	if err := r.Get(ctx, req.NamespacedName, &monitoring); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Monitoring resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Monitoring")
		return ctrl.Result{}, err
	}

	// Edge 필드 순회 및 splitPoint 지역 변수 선언
	splitPoint := split_point()

	// Prometheus URL 설정
	//prometheusURL := "http://prometheus-karmada-url/api/v1"

	// Edge 클러스터 CPU 사용량을 확인하고 AutoDeploy 생성
	for _, edge := range monitoring.Spec.Edge {

		image := fmt.Sprintf("bono1130/head-model-%d", splitPoint)
		autoDeploy := CreateAutoDeploy(image, edge)
		if err := r.Create(ctx, autoDeploy); err != nil {
			log.Error(err, "Failed to create Head AutoDeploy")
			return reconcile.Result{}, err
		}
	}

	// Core 필드를 보고 Tail AutoDeploy 생성
	image := fmt.Sprintf("bono1130/tail-model-%d", splitPoint)
	tailDeploy := CreateAutoDeploy(image, monitoring.Spec.Core)
	if err := r.Create(ctx, tailDeploy); err != nil {
		log.Error(err, "Failed to create Tail AutoDeploy")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MonitoringReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.Monitoring{}).
		Complete(r)
}
