package controller

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	backupschemav1 "github.com/cyse7125-sp25-team02/db-backup-operator/api/v1"
)

// BackupDatabaseSchemaReconciler reconciles a BackupDatabaseSchema object
type BackupDatabaseSchemaReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=backupschema.jkops.me,resources=backupdatabaseschemas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=backupschema.jkops.me,resources=backupdatabaseschemas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=backupschema.jkops.me,resources=backupdatabaseschemas/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

func (r *BackupDatabaseSchemaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	backup := &backupschemav1.BackupDatabaseSchema{}
	if err := r.Get(ctx, req.NamespacedName, backup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cronJobName := fmt.Sprintf("backup-%s", backup.Name)
	cronJob := &batchv1.CronJob{}
	cronJobKey := types.NamespacedName{Name: cronJobName, Namespace: backup.Spec.BackupJobNamespace}
	if err := r.Get(ctx, cronJobKey, cronJob); err != nil && client.IgnoreNotFound(err) == nil {
		newCronJob, err := r.createBackupCronJob(backup)
		if err != nil {
			log.Error(err, "Failed to create backup CronJob")
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, newCronJob); err != nil {
			log.Error(err, "Failed to create backup CronJob")
			return ctrl.Result{}, err
		}
		log.Info("Created backup CronJob", "cronJobName", cronJobName)
	} else if err == nil {
		desiredCronJob, err := r.createBackupCronJob(backup)
		if err != nil {
			log.Error(err, "Failed to generate desired CronJob spec")
			return ctrl.Result{}, err
		}
		if !reflect.DeepEqual(cronJob.Spec, desiredCronJob.Spec) {
			cronJob.Spec = desiredCronJob.Spec
			if err := r.Update(ctx, cronJob); err != nil {
				log.Error(err, "Failed to update backup CronJob")
				return ctrl.Result{}, err
			}
			log.Info("Updated backup CronJob", "cronJobName", cronJobName)
		}
	}

	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList, client.InNamespace(backup.Spec.BackupJobNamespace), client.MatchingLabels{"cronjob-name": cronJobName}); err != nil {
		log.Error(err, "Failed to list jobs")
		return ctrl.Result{}, err
	}

	sort.Slice(jobList.Items, func(i, j int) bool {
		if jobList.Items[i].Status.CompletionTime == nil {
			return true
		}
		if jobList.Items[j].Status.CompletionTime == nil {
			return false
		}
		return jobList.Items[i].Status.CompletionTime.Time.Before(jobList.Items[j].Status.CompletionTime.Time)
	})

	for _, job := range jobList.Items {
		log.Info("Processing job", "jobName", job.Name, "Job Status", job.Status)

		if job.Status.CompletionTime == nil {
			continue
		}

		log.Info("Processing job", "jobName", job.Name)

		lastTime, err := mustParseTime(backup.Status.LastBackupTime)
		if backup.Status.LastBackupTime == "" || (err == nil && job.Status.CompletionTime.Time.After(lastTime)) {
			backup.Status.LastBackupTime = job.Status.CompletionTime.Time.UTC().Format(time.RFC3339)
			backup.Status.LastBackupJob = job.Name

			if job.Status.Succeeded > 0 {
				backup.Status.BackupStatus = "Success"
				
				parts := strings.Split(job.Name, "-")
				if len(parts) > 0 {
					timestamp := parts[len(parts)-1]
					fileName := fmt.Sprintf("%s.sql", timestamp)
					backup.Status.BackupLocation = fmt.Sprintf("gs://%s/%s", backup.Spec.GCSBucket, fileName)
				}
			} else {
				backup.Status.BackupStatus = "Failed"
				backup.Status.BackupLocation = ""
			}

			if err := r.Status().Update(ctx, backup); err != nil {
				log.Error(err, "Failed to update BackupDatabaseSchema status")
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
}

func (r *BackupDatabaseSchemaReconciler) createBackupCronJob(backup *backupschemav1.BackupDatabaseSchema) (*batchv1.CronJob, error) {
	interval := backup.Spec.BackupInterval
	cronSchedule := fmt.Sprintf("*/%d * * * *", interval)

	// Use JOB_NAME and strip pod suffix to match job name
	backupCommand := fmt.Sprintf(
		"export JOB_NAME=$(echo $POD_NAME | sed 's/-[a-z0-9]*$//'); timestamp=$(echo $JOB_NAME | awk -F'-' '{print $NF}'); pg_dump -h %s -p %d -U %s -n %s %s | gsutil cp - gs://%s/$timestamp.sql",
		backup.Spec.DBHost, backup.Spec.DBPort, backup.Spec.DBUser,
		backup.Spec.DBSchema, backup.Spec.DBName, backup.Spec.GCSBucket,
	)

	container := corev1.Container{
		Name:    "backup",
		Image:   "karanthakkar09/controller:latest",
		Command: []string{"/bin/sh", "-c"},
		Args:    []string{backupCommand},
		Env: []corev1.EnvVar{
			{
				Name: "PGPASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: backup.Spec.DBPasswordSecretName,
						},
						Key: backup.Spec.DBPasswordSecretKey,
					},
				},
			},
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
		},
	}

	if backup.Spec.GCPServiceAccountSecretName != "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "GOOGLE_APPLICATION_CREDENTIALS",
			Value: "/var/secrets/gcp/key.json",
		})
		container.VolumeMounts = []corev1.VolumeMount{
			{
				Name:      "gcp-secret",
				MountPath: "/var/secrets/gcp",
				ReadOnly:  true,
			},
		}
	}

	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("backup-%s", backup.Name),
			Namespace: backup.Spec.BackupJobNamespace,
			Labels:    map[string]string{"cronjob-name": fmt.Sprintf("backup-%s", backup.Name)},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   cronSchedule,
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			SuccessfulJobsHistoryLimit: ptr.To[int32](10),
			FailedJobsHistoryLimit:     ptr.To[int32](10),
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"cronjob-name": fmt.Sprintf("backup-%s", backup.Name)},
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: backup.Spec.KubeServiceAccount,
							RestartPolicy:      corev1.RestartPolicyNever,
							Containers:         []corev1.Container{container},
						},
					},
				},
			},
		},
	}

	if backup.Spec.GCPServiceAccountSecretName != "" {
		cronJob.Spec.JobTemplate.Spec.Template.Spec.Volumes = []corev1.Volume{
			{
				Name: "gcp-secret",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: backup.Spec.GCPServiceAccountSecretName,
					},
				},
			},
		}
	}

	if err := ctrl.SetControllerReference(backup, cronJob, r.Scheme); err != nil {
		return nil, err
	}

	return cronJob, nil
}

func mustParseTime(timeStr string) (time.Time, error) {
	return time.Parse(time.RFC3339, timeStr)
}

func (r *BackupDatabaseSchemaReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&backupschemav1.BackupDatabaseSchema{}).
        Owns(&batchv1.CronJob{}).
        Owns(&batchv1.Job{}).
        Complete(r)
}
