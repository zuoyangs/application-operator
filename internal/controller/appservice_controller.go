package controller

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appv1 "github.com/zuoyangs/application-operator/api/v1"
)

// AppServiceReconciler reconciles a AppService object
type AppServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=app.example.com,resources=appservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=app.example.com,resources=appservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=app.example.com,resources=appservices/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete  // 新增：允许 Controller 操作 Pod
//+kubebuilder:rbac:groups=core,resources=pods/status,verbs=get  // 新增：允许 Controller 查看 Pod 状态

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AppService object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *AppServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// 1. 获取当前触发 Reconcile 的 AppService CR 实例
	var appService appv1.AppService
	if err := r.Get(ctx, req.NamespacedName, &appService); err != nil {
		log.Error(err, "unable to fetch AppService")
		// 如果 CR 不存在（比如被删除），直接返回，忽略错误
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. 定义期望创建的 Pod 模板（根据 CR 的 Spec 字段）
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appService.Name + "-pod", // Pod 名称 = CR 名称 + "-pod"（自定义，保证唯一）
			Namespace: appService.Namespace,     // Pod 命名空间和 CR 一致
			Labels: map[string]string{
				"app": appService.Name, // 给 Pod 打标签，方便后续管理
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app-container",       // 容器名称（自定义）
					Image: appService.Spec.Image, // 容器镜像 = CR Spec 中的 Image
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: appService.Spec.Port, // 容器端口 = CR Spec 中的 Port
						},
					},
				},
			},
		},
	}

	// 3. 关联 Pod 和 CR（让 Kubernetes 知道这个 Pod 属于这个 CR）
	if err := ctrl.SetControllerReference(&appService, pod, r.Scheme); err != nil {
		log.Error(err, "unable to set controller reference on Pod")
		return ctrl.Result{}, err
	}

	// 4. 检查 Pod 是否已经存在
	var existingPod corev1.Pod
	if err := r.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, &existingPod); err != nil {
		// 4.1 如果 Pod 不存在，创建 Pod
		if errors.IsNotFound(err) {
			log.Info("creating new Pod", "pod", pod.Name)
			if err := r.Create(ctx, pod); err != nil {
				log.Error(err, "unable to create Pod")
				return ctrl.Result{}, err
			}
			// Pod 创建成功，返回 Requeue，重新检查状态
			return ctrl.Result{Requeue: true}, nil
		}
		// 4.2 如果检查 Pod 时出现其他错误，返回错误
		log.Error(err, "unable to fetch existing Pod")
		return ctrl.Result{}, err
	}

	// 5. 如果 Pod 已经存在，更新 CR 的 Status 字段（记录实际状态）
	appService.Status.CurrentReplicas = 1 // 这里简化处理，实际场景中需要统计 Pod 数量
	appService.Status.Status = "Running"  // 标记服务状态为 Running
	if err := r.Status().Update(ctx, &appService); err != nil {
		log.Error(err, "unable to update AppService status")
		return ctrl.Result{}, err
	}

	// 6. 期望状态和实际状态一致，返回不重新触发 Reconcile
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1.AppService{}). // 监听 AppService 类型的 CR
		Owns(&corev1.Pod{}).      // 监听和 CR 关联的 Pod 资源
		Complete(r)
}
