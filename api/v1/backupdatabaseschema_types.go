package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupDatabaseSchemaSpec defines the desired state of BackupDatabaseSchema
type BackupDatabaseSchemaSpec struct {
	// DBHost is the hostname or IP of the database server.
	DBHost string `json:"dbHost"`
	// DBUser is the username for the database.
	DBUser string `json:"dbUser"`
	// DBPasswordSecretName is the name of the Kubernetes Secret containing the database password.
	DBPasswordSecretName string `json:"dbPasswordSecretName"`
	// DBPasswordSecretNamespace is the namespace of the Kubernetes Secret containing the database password.
	DBPasswordSecretNamespace string `json:"dbPasswordSecretNamespace"`
	// DBPasswordSecretKey is the key in the Kubernetes Secret that holds the database password.
	DBPasswordSecretKey string `json:"dbPasswordSecretKey"`
	// DBName is the name of the database.
	DBName string `json:"dbName"`
	// DBSchema is the schema name to back up.
	DBSchema string `json:"dbSchema"`
	// DBPort is the port number of the database server.
	DBPort int32 `json:"dbPort"`
	// GCSBucket is the name of the Google Cloud Storage bucket to store the backup.
	GCSBucket string `json:"gcsBucket"`
	// KubeServiceAccount is the Kubernetes ServiceAccount to use for running the backup workload.
	KubeServiceAccount string `json:"kubeServiceAccount"`
	// GCPServiceAccount is the GCP ServiceAccount with access to the GCS bucket.
	GCPServiceAccount string `json:"gcpServiceAccount"`
	// BackupJobNamespace is the Kubernetes namespace where the operator runs the backup job.
	BackupJobNamespace string `json:"backupJobNamespace"`
	// GCPServiceAccountSecretName is the name of the Kubernetes Secret containing the GCP Service Account key (optional).
	GCPServiceAccountSecretName string `json:"gcpServiceAccountSecretName,omitempty"`
	// BackupInterval is the interval in minutes between backups (e.g., 60 for hourly).
	BackupInterval int32 `json:"backupInterval"`
}

// BackupDatabaseSchemaStatus defines the observed state of BackupDatabaseSchema
type BackupDatabaseSchemaStatus struct {
	// LastBackupTime is the UTC time when the last backup job was run.
	LastBackupTime string `json:"lastBackupTime,omitempty"`
	// BackupLocation is the full location of the backup in the GCS bucket.
	BackupLocation string `json:"backupLocation,omitempty"`
	// BackupStatus is the status of the backup job (e.g., "Success", "Failed", "Running").
	BackupStatus string `json:"backupStatus,omitempty"`
	// LastBackupJob is the name of the most recent backup job.
	LastBackupJob string `json:"lastBackupJob,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="LastBackupTime",type="string",JSONPath=".status.lastBackupTime"
// +kubebuilder:printcolumn:name="BackupStatus",type="string",JSONPath=".status.backupStatus"
// +kubebuilder:resource:scope=Namespaced

// BackupDatabaseSchema is the Schema for the backupdatabaseschemas API
type BackupDatabaseSchema struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupDatabaseSchemaSpec   `json:"spec,omitempty"`
	Status BackupDatabaseSchemaStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BackupDatabaseSchemaList contains a list of BackupDatabaseSchema
type BackupDatabaseSchemaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupDatabaseSchema `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupDatabaseSchema{}, &BackupDatabaseSchemaList{})
}
